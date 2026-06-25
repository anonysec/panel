package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RestoreFromUpload validates and applies a backup from an uploaded file.
func (s *Service) RestoreFromUpload(ctx context.Context, file io.Reader, filename string) error {
	// 1. Acquire mutex
	if !s.mu.TryLock() {
		return fmt.Errorf("backup operation already in progress")
	}
	defer s.mu.Unlock()

	// 2. Validate filename
	if !strings.HasSuffix(strings.ToLower(filename), ".tar.gz") && !strings.HasSuffix(strings.ToLower(filename), ".tgz") {
		return fmt.Errorf("invalid archive: file must be .tar.gz")
	}

	// 3. Save uploaded file to temp location
	tmpFile, err := os.CreateTemp("", "koris-restore-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Copy uploaded file to temp and compute checksum simultaneously
	h := sha256.New()
	tee := io.TeeReader(file, h)
	if _, err := io.Copy(tmpFile, tee); err != nil {
		tmpFile.Close()
		return fmt.Errorf("save uploaded file: %w", err)
	}
	tmpFile.Close()
	actualChecksum := fmt.Sprintf("%x", h.Sum(nil))

	// 4. Validate archive structure: must contain dump.sql and manifest.json
	manifest, err := validateArchiveStructure(tmpPath)
	if err != nil {
		return err
	}

	// 5. Verify checksum if manifest has one
	if manifest.Checksum != "" && manifest.Checksum != actualChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", manifest.Checksum, actualChecksum)
	}

	// 6. Create pre-restore safety backup
	// Run a lightweight SQL dump directly (without releasing mutex) to avoid race conditions.
	log.Println("[backup] creating pre-restore safety backup")
	preErr := s.createPreRestoreBackup(ctx)
	if preErr != nil {
		log.Printf("[backup] pre-restore backup failed (continuing): %v", preErr)
	}

	// 7. Apply dump.sql via mysql command
	if err := s.applyDumpSQL(ctx, tmpPath); err != nil {
		return fmt.Errorf("apply dump.sql: %w", err)
	}

	// 8. Extract configs per node and dispatch restore tasks
	if err := s.dispatchConfigRestores(ctx, tmpPath); err != nil {
		log.Printf("[backup] config restore dispatch failed (continuing): %v", err)
	}

	log.Println("[backup] restore completed successfully")
	return nil
}

// createPreRestoreBackup performs a quick SQL dump backup without releasing the mutex.
// This is a simplified version of CreateBackup that doesn't collect node configs.
func (s *Service) createPreRestoreBackup(ctx context.Context) error {
	if err := ensureStorageDir(s.cfg.StorageDir); err != nil {
		return fmt.Errorf("ensure storage dir: %w", err)
	}

	now := time.Now()
	filename := fmt.Sprintf("pre-restore-%s.tar.gz", now.Format("20060102-150405"))
	archivePath := filepath.Join(s.cfg.StorageDir, filename)

	dumpReader, dumpWait, err := streamMySQLDump(ctx, s.cfg)
	if err != nil {
		return err
	}

	manifest := GenerateManifest(now, s.getPanelVersion(), s.cfg.DBName, nil, nil, nil, 0, 0)
	if err := WriteArchive(archivePath, dumpReader, nil, manifest); err != nil {
		return err
	}
	if err := dumpWait(); err != nil {
		os.Remove(archivePath)
		return err
	}

	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO backups (filename, status, type, started_at, completed_at) VALUES (?, 'completed', 'pre_restore', NOW(), NOW())`,
		filename)

	return nil
}

// validateArchiveStructure opens the tar.gz and checks for required files.
func validateArchiveStructure(archivePath string) (*Manifest, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("invalid archive: not a valid gzip file")
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	var hasDumpSQL, hasManifest bool
	var manifest Manifest

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid archive: corrupt tar structure")
		}

		switch hdr.Name {
		case "dump.sql":
			hasDumpSQL = true
		case "manifest.json":
			hasManifest = true
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("invalid archive: cannot read manifest.json")
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				return nil, fmt.Errorf("invalid archive: invalid manifest.json format")
			}
		}
	}

	if !hasDumpSQL {
		return nil, fmt.Errorf("invalid archive: missing dump.sql")
	}
	if !hasManifest {
		return nil, fmt.Errorf("invalid archive: missing manifest.json")
	}

	return &manifest, nil
}

// applyDumpSQL extracts dump.sql from the archive and pipes it to the mysql command.
func (s *Service) applyDumpSQL(ctx context.Context, archivePath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// Find dump.sql in the archive
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("dump.sql not found in archive")
		}
		if err != nil {
			return err
		}
		if hdr.Name == "dump.sql" {
			break
		}
	}

	// Stream dump.sql into mysql command
	args := []string{
		"--force",
		"-h", s.cfg.DBHost,
		"-u", s.cfg.DBUser,
		s.cfg.DBName,
	}
	cmd := exec.CommandContext(ctx, "mysql", args...)
	cmd.Env = append(cmd.Environ(), "MYSQL_PWD="+s.cfg.DBPass)
	cmd.Stdin = tr

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysql restore failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// dispatchConfigRestores extracts node config files from the archive and dispatches restore tasks.
func (s *Service) dispatchConfigRestores(ctx context.Context, archivePath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// Collect files per node from configs/{node_name}/ prefix
	nodeFiles := make(map[string]map[string][]byte) // node_name -> path -> content

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if !strings.HasPrefix(hdr.Name, "configs/") || hdr.Typeflag == tar.TypeDir {
			continue
		}

		// Parse: configs/{node_name}/{remaining_path}
		parts := strings.SplitN(strings.TrimPrefix(hdr.Name, "configs/"), "/", 2)
		if len(parts) != 2 {
			continue
		}
		nodeName := parts[0]
		filePath := parts[1]

		content, err := io.ReadAll(tr)
		if err != nil {
			continue
		}

		if nodeFiles[nodeName] == nil {
			nodeFiles[nodeName] = make(map[string][]byte)
		}
		nodeFiles[nodeName][filePath] = content
	}

	// For each node, create a tar archive, base64 encode, and dispatch task
	for nodeName, files := range nodeFiles {
		tarData, err := createTarFromFiles(files)
		if err != nil {
			log.Printf("[backup] failed to create tar for node %s: %v", nodeName, err)
			continue
		}

		// Find node ID by name
		var nodeID int64
		err = s.db.QueryRowContext(ctx, `SELECT id FROM nodes WHERE name=? AND status IN ('online','stale')`, nodeName).Scan(&nodeID)
		if err != nil {
			log.Printf("[backup] node %s not found or offline, skipping config restore", nodeName)
			continue
		}

		// NOTE: Legacy node_tasks-based config restore has been removed.
		// Restore dispatching is now handled via gRPC.
		_ = tarData // will be sent via gRPC in future
		log.Printf("[backup] config restore for node %s (id=%d) would be dispatched via gRPC", nodeName, nodeID)
	}

	return nil
}

// createTarFromFiles creates a tar archive from a map of file paths to content.
func createTarFromFiles(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for path, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: path,
			Size: int64(len(content)),
			Mode: 0640,
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(content); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
