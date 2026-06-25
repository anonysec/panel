package backup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"KorisPanel/panel/internal/notify"
)

// Config holds backup service configuration loaded from panel_settings.
type Config struct {
	StorageDir     string
	Schedule       string
	RetentionCount int
	DBUser         string
	DBPass         string
	DBName         string
	DBHost         string
}

// BackupRecord represents a row in the backups table.
type BackupRecord struct {
	ID            int64
	Filename      string
	Status        string
	Type          string
	SizeBytes     sql.NullInt64
	Checksum      sql.NullString
	NodesIncluded sql.NullString
	NodesSkipped  sql.NullString
	ErrorMessage  sql.NullString
	StartedAt     time.Time
	CompletedAt   sql.NullTime
}

// Service manages backup creation, scheduling, retention, and restore operations.
type Service struct {
	db       *sql.DB
	cfg      Config
	mu       sync.Mutex
	notifier *notify.Notifier
}

// New creates a new backup Service with the given database connection and config.
func New(db *sql.DB, cfg Config, notifier ...*notify.Notifier) *Service {
	s := &Service{
		db:  db,
		cfg: cfg,
	}
	if len(notifier) > 0 {
		s.notifier = notifier[0]
	}
	return s
}

// LoadConfigFromDB reads backup_schedule and backup_retention_count from panel_settings
// and extracts DB credentials from the PANEL_DB_DSN environment variable.
func LoadConfigFromDB(db *sql.DB) Config {
	cfg := Config{
		StorageDir:     "/opt/KorisPanel/backups/",
		Schedule:       "daily:02",
		RetentionCount: 7,
	}

	rows, err := db.Query(`SELECT setting_key, setting_value FROM panel_settings WHERE setting_key IN ('backup_schedule', 'backup_retention_count')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var key, val string
			if rows.Scan(&key, &val) == nil {
				switch key {
				case "backup_schedule":
					cfg.Schedule = val
				case "backup_retention_count":
					if n, err := strconv.Atoi(val); err == nil && n > 0 {
						cfg.RetentionCount = n
					}
				}
			}
		}
	}

	dsn := os.Getenv("PANEL_DB_DSN")
	cfg.DBUser, cfg.DBPass, cfg.DBName, cfg.DBHost = mysqlCredsFromDSN(dsn)
	if cfg.DBName == "" {
		cfg.DBName = "radius_next"
	}
	if cfg.DBHost == "" {
		cfg.DBHost = "localhost"
	}

	return cfg
}

// UpdateConfig persists schedule and retention count to panel_settings.
func (s *Service) UpdateConfig(schedule string, retentionCount int) error {
	_, err := s.db.Exec(`UPDATE panel_settings SET setting_value = ? WHERE setting_key = 'backup_schedule'`, schedule)
	if err != nil {
		return fmt.Errorf("update backup_schedule: %w", err)
	}
	_, err = s.db.Exec(`UPDATE panel_settings SET setting_value = ? WHERE setting_key = 'backup_retention_count'`, strconv.Itoa(retentionCount))
	if err != nil {
		return fmt.Errorf("update backup_retention_count: %w", err)
	}
	s.cfg.Schedule = schedule
	s.cfg.RetentionCount = retentionCount
	return nil
}

// mysqlCredsFromDSN extracts user, password, database name, and host from a MySQL DSN string.
// DSN format: user:pass@tcp(host:port)/dbname?params
func mysqlCredsFromDSN(dsn string) (user, pass, dbName, host string) {
	at := strings.Index(dsn, "@")
	if at == -1 {
		return "", "", "", ""
	}

	// Parse credentials (user:pass)
	creds := dsn[:at]
	colon := strings.Index(creds, ":")
	if colon != -1 {
		user = creds[:colon]
		pass = creds[colon+1:]
	} else {
		user = creds
	}

	// Parse host and database from remainder after @
	remainder := dsn[at+1:]

	// Extract host from tcp(host:port) or host:port
	if strings.HasPrefix(remainder, "tcp(") {
		end := strings.Index(remainder, ")")
		if end != -1 {
			hostPort := remainder[4:end]
			if colonIdx := strings.Index(hostPort, ":"); colonIdx != -1 {
				host = hostPort[:colonIdx]
			} else {
				host = hostPort
			}
			remainder = remainder[end+1:]
		}
	}

	// Extract database name from /dbname?params
	if strings.HasPrefix(remainder, "/") {
		remainder = remainder[1:]
	}
	if i := strings.Index(remainder, "?"); i != -1 {
		dbName = remainder[:i]
	} else {
		dbName = remainder
	}

	return
}

// CreateBackup initiates a full backup: SQL dump + node configs + archive packaging.
// Returns the backup record ID.
func (s *Service) CreateBackup(ctx context.Context, backupType string) (int64, error) {
	// 1. Acquire mutex (concurrent prevention)
	if !s.mu.TryLock() {
		return 0, fmt.Errorf("backup already in progress")
	}
	defer s.mu.Unlock()

	startTime := time.Now()

	// 2. Ensure storage directory exists
	if err := ensureStorageDir(s.cfg.StorageDir); err != nil {
		return 0, fmt.Errorf("ensure storage dir: %w", err)
	}

	// 3. Insert backup record with status "in_progress"
	now := time.Now()
	filename := generateFilename(now)
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (filename, status, type, started_at) VALUES (?, 'in_progress', ?, NOW())`,
		filename, backupType)
	if err != nil {
		return 0, fmt.Errorf("insert backup record: %w", err)
	}
	backupID, _ := result.LastInsertId()

	// 4. Execute mysqldump (streaming)
	dumpReader, dumpWait, err := streamMySQLDump(ctx, s.cfg)
	if err != nil {
		s.failBackup(ctx, backupID, err.Error())
		if backupType == "scheduled" && s.notifier != nil {
			s.notifier.Send(formatBackupFailure(filename, err.Error()))
		}
		return backupID, err
	}

	// 5. Collect node configs (query online nodes, dispatch tasks, wait with timeout)
	nodesIncluded, nodesSkipped, nodeConfigs := s.collectNodeConfigs(ctx)

	// 6. Generate manifest (counts will be filled after scanning dump)
	manifest := GenerateManifest(now, s.getPanelVersion(), s.cfg.DBName, nodesIncluded, nodesSkipped, nil, 0, 0)

	// 7. Write archive
	archivePath := filepath.Join(s.cfg.StorageDir, filename)
	if err := WriteArchive(archivePath, dumpReader, nodeConfigs, manifest); err != nil {
		s.failBackup(ctx, backupID, err.Error())
		if backupType == "scheduled" && s.notifier != nil {
			s.notifier.Send(formatBackupFailure(filename, err.Error()))
		}
		return backupID, err
	}

	// 8. Wait for mysqldump to finish (check exit code)
	if err := dumpWait(); err != nil {
		s.failBackup(ctx, backupID, err.Error())
		os.Remove(archivePath)
		if backupType == "scheduled" && s.notifier != nil {
			s.notifier.Send(formatBackupFailure(filename, err.Error()))
		}
		return backupID, err
	}

	// 8.5. Scan the dump file in the archive for table/row counts and rewrite manifest
	tableCount, totalRowCount := countTablesAndRows(archivePath)
	if tableCount > 0 || totalRowCount > 0 {
		manifest.TableCount = tableCount
		manifest.TotalRowCount = totalRowCount
		// Rewrite the archive with updated manifest is not practical;
		// we update the manifest in-place via the stored counts for preview purposes.
	}

	// 9. Compute checksum
	checksum, err := ComputeChecksum(archivePath)
	if err != nil {
		s.failBackup(ctx, backupID, err.Error())
		if backupType == "scheduled" && s.notifier != nil {
			s.notifier.Send(formatBackupFailure(filename, err.Error()))
		}
		return backupID, err
	}
	WriteChecksumFile(archivePath, checksum)

	// 10. Get file size
	fi, _ := os.Stat(archivePath)
	sizeBytes := fi.Size()

	// 11. Update record to completed
	_, _ = s.db.ExecContext(ctx,
		`UPDATE backups SET status='completed', size_bytes=?, checksum=?, nodes_included=?, nodes_skipped=?, completed_at=NOW() WHERE id=?`,
		sizeBytes, checksum, marshalJSON(nodesIncluded), marshalJSON(nodesSkipped), backupID)

	// 12. Apply retention policy
	s.ApplyRetention(ctx)

	// 13. Send success notification for scheduled backups
	if backupType == "scheduled" && s.notifier != nil {
		duration := time.Since(startTime)
		s.notifier.Send(formatBackupSuccess(filename, sizeBytes, duration, len(nodesIncluded), len(nodesSkipped)))
	}

	return backupID, nil
}

// failBackup updates a backup record to "failed" with the given error message.
func (s *Service) failBackup(ctx context.Context, id int64, errMsg string) {
	_, _ = s.db.ExecContext(ctx,
		`UPDATE backups SET status='failed', error_message=?, completed_at=NOW() WHERE id=?`,
		errMsg, id)
}

// collectNodeConfigs queries online nodes, dispatches backup.collect_configs tasks,
// and waits up to 60s for responses. Returns included node names, skipped nodes, and configs.
func (s *Service) collectNodeConfigs(ctx context.Context) ([]string, []SkippedNode, []NodeConfigs) {
	var nodesIncluded []string
	var nodesSkipped []SkippedNode
	var nodeConfigs []NodeConfigs

	// Query online/stale nodes
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM nodes WHERE status IN ('online','stale')`)
	if err != nil {
		return nodesIncluded, nodesSkipped, nodeConfigs
	}
	defer rows.Close()

	type nodeInfo struct {
		id   int64
		name string
	}
	var nodes []nodeInfo
	for rows.Next() {
		var n nodeInfo
		if rows.Scan(&n.id, &n.name) == nil {
			nodes = append(nodes, n)
		}
	}

	if len(nodes) == 0 {
		return nodesIncluded, nodesSkipped, nodeConfigs
	}

	// NOTE: Legacy node_tasks-based backup.collect_configs has been removed.
	// Node config collection is now handled via gRPC calls.
	// For now, skip node config collection until gRPC backup integration is complete.
	for _, n := range nodes {
		log.Printf("[backup] node config collection for %s would be dispatched via gRPC", n.name)
		nodesSkipped = append(nodesSkipped, SkippedNode{Name: n.name, Reason: "grpc_backup_not_yet_wired"})
	}

	return nodesIncluded, nodesSkipped, nodeConfigs
}

// parseNodeConfigResult parses the result_json from a backup.collect_configs task.
// Expected format: {"configs_tar_base64": "...", "files_count": N, "total_size": M}
func parseNodeConfigResult(nodeName, resultJSON string) *NodeConfigs {
	var result struct {
		ConfigsTarBase64 string `json:"configs_tar_base64"`
		FilesCount       int    `json:"files_count"`
		TotalSize        int64  `json:"total_size"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return nil
	}
	if result.ConfigsTarBase64 == "" {
		return nil
	}

	// Decode base64 tar and extract files
	files, err := decodeTarBase64(result.ConfigsTarBase64)
	if err != nil {
		return nil
	}

	return &NodeConfigs{
		NodeName: nodeName,
		Files:    files,
	}
}

// decodeTarBase64 decodes a base64-encoded tar archive and returns a map of path -> content.
func decodeTarBase64(encoded string) (map[string][]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	return extractTarBytes(data)
}

// extractTarBytes extracts files from a tar byte slice into a map of path -> content.
func extractTarBytes(data []byte) (map[string][]byte, error) {
	files := make(map[string][]byte)
	tr := tar.NewReader(bytes.NewReader(data))

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry: %w", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read tar content %s: %w", hdr.Name, err)
		}
		files[hdr.Name] = content
	}

	return files, nil
}

// getPanelVersion reads PANEL_VERSION env or returns "dev".
func (s *Service) getPanelVersion() string {
	if v := os.Getenv("PANEL_VERSION"); v != "" {
		return v
	}
	return "dev"
}

// ApplyRetention counts completed backups and deletes the oldest beyond the retention limit.
func (s *Service) ApplyRetention(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, filename FROM backups WHERE status='completed' ORDER BY started_at DESC`)
	if err != nil {
		return
	}
	defer rows.Close()

	type record struct {
		id       int64
		filename string
	}
	var records []record
	for rows.Next() {
		var r record
		if rows.Scan(&r.id, &r.filename) == nil {
			records = append(records, r)
		}
	}

	if len(records) <= s.cfg.RetentionCount {
		return
	}

	// Delete oldest beyond retention limit
	for _, r := range records[s.cfg.RetentionCount:] {
		archivePath := filepath.Join(s.cfg.StorageDir, r.filename)
		os.Remove(archivePath)
		os.Remove(archivePath + ".sha256")
		_, _ = s.db.ExecContext(ctx,
			`DELETE FROM backups WHERE id=?`, r.id)
	}
}

// StorageDir returns the configured backup storage directory path.
func (s *Service) StorageDir() string {
	return s.cfg.StorageDir
}

// CreateBackupAsync inserts a backup record and returns its ID immediately.
// The caller should then call RunBackup in a goroutine.
func (s *Service) CreateBackupAsync(ctx context.Context) (int64, error) {
	if !s.mu.TryLock() {
		return 0, fmt.Errorf("backup already in progress")
	}
	s.mu.Unlock()

	// Ensure storage directory exists
	if err := ensureStorageDir(s.cfg.StorageDir); err != nil {
		return 0, fmt.Errorf("ensure storage dir: %w", err)
	}

	now := time.Now()
	filename := generateFilename(now)
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (filename, status, type, started_at) VALUES (?, 'in_progress', 'manual', NOW())`,
		filename)
	if err != nil {
		return 0, fmt.Errorf("insert backup record: %w", err)
	}
	return result.LastInsertId()
}

// RunBackup performs the actual backup operation for a pre-created record.
func (s *Service) RunBackup(ctx context.Context, backupID int64, backupType string) {
	if !s.mu.TryLock() {
		s.failBackup(ctx, backupID, "backup already in progress")
		return
	}
	defer s.mu.Unlock()

	startTime := time.Now()

	// Get filename from record
	var filename string
	if err := s.db.QueryRowContext(ctx, `SELECT filename FROM backups WHERE id=?`, backupID).Scan(&filename); err != nil {
		s.failBackup(ctx, backupID, "record not found: "+err.Error())
		return
	}

	// Update type
	_, _ = s.db.ExecContext(ctx, `UPDATE backups SET type=? WHERE id=?`, backupType, backupID)

	// Execute mysqldump
	dumpReader, dumpWait, err := streamMySQLDump(ctx, s.cfg)
	if err != nil {
		s.failBackup(ctx, backupID, err.Error())
		if backupType == "scheduled" && s.notifier != nil {
			s.notifier.Send(formatBackupFailure(filename, err.Error()))
		}
		return
	}

	// Collect node configs
	nodesIncluded, nodesSkipped, nodeConfigs := s.collectNodeConfigs(ctx)

	// Generate manifest
	manifest := GenerateManifest(time.Now(), s.getPanelVersion(), s.cfg.DBName, nodesIncluded, nodesSkipped, nil, 0, 0)

	// Write archive
	archivePath := filepath.Join(s.cfg.StorageDir, filename)
	if err := WriteArchive(archivePath, dumpReader, nodeConfigs, manifest); err != nil {
		s.failBackup(ctx, backupID, err.Error())
		if backupType == "scheduled" && s.notifier != nil {
			s.notifier.Send(formatBackupFailure(filename, err.Error()))
		}
		return
	}

	// Wait for mysqldump
	if err := dumpWait(); err != nil {
		s.failBackup(ctx, backupID, err.Error())
		os.Remove(archivePath)
		if backupType == "scheduled" && s.notifier != nil {
			s.notifier.Send(formatBackupFailure(filename, err.Error()))
		}
		return
	}

	// Scan for table/row counts
	tableCount, totalRowCount := countTablesAndRows(archivePath)
	_ = tableCount
	_ = totalRowCount

	// Compute checksum
	checksum, err := ComputeChecksum(archivePath)
	if err != nil {
		s.failBackup(ctx, backupID, err.Error())
		if backupType == "scheduled" && s.notifier != nil {
			s.notifier.Send(formatBackupFailure(filename, err.Error()))
		}
		return
	}
	WriteChecksumFile(archivePath, checksum)

	// Get file size
	fi, _ := os.Stat(archivePath)
	sizeBytes := fi.Size()

	// Update record to completed
	_, _ = s.db.ExecContext(ctx,
		`UPDATE backups SET status='completed', size_bytes=?, checksum=?, nodes_included=?, nodes_skipped=?, completed_at=NOW() WHERE id=?`,
		sizeBytes, checksum, marshalJSON(nodesIncluded), marshalJSON(nodesSkipped), backupID)

	// Apply retention policy
	s.ApplyRetention(ctx)

	// Send success notification for scheduled backups
	if backupType == "scheduled" && s.notifier != nil {
		duration := time.Since(startTime)
		s.notifier.Send(formatBackupSuccess(filename, sizeBytes, duration, len(nodesIncluded), len(nodesSkipped)))
	}
}

// marshalJSON is a helper for JSON column encoding.
func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// countTablesAndRows scans the dump.sql inside a tar.gz archive for CREATE TABLE statements
// and counts the VALUES tuples in INSERT statements to estimate row counts.
func countTablesAndRows(archivePath string) (tableCount int, totalRowCount int64) {
	f, err := os.Open(archivePath)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return 0, 0
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// Find dump.sql
	for {
		hdr, err := tr.Next()
		if err != nil {
			return 0, 0
		}
		if hdr.Name == "dump.sql" {
			break
		}
	}

	// Scan dump.sql line by line
	createTableRe := regexp.MustCompile(`(?i)^CREATE\s+TABLE\s`)
	insertRe := regexp.MustCompile(`(?i)^INSERT\s+INTO\s`)

	scanner := bufio.NewScanner(tr)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer for large INSERT statements

	for scanner.Scan() {
		line := scanner.Text()
		if createTableRe.MatchString(line) {
			tableCount++
		} else if insertRe.MatchString(line) {
			// Count VALUES tuples: each '),(' or the opening '(' after VALUES
			valuesIdx := strings.Index(strings.ToUpper(line), "VALUES")
			if valuesIdx >= 0 {
				valuesPart := line[valuesIdx:]
				// Count the number of opening parentheses that start a row
				// Each row is wrapped in (...), separated by commas
				totalRowCount += int64(strings.Count(valuesPart, "),(")) + 1
			}
		}
	}
	// Ignore scanner.Err() — partial counts are acceptable

	return tableCount, totalRowCount
}

// formatBackupSuccess formats a Telegram notification message for a successful scheduled backup.
func formatBackupSuccess(filename string, sizeBytes int64, duration time.Duration, nodesIncluded int, nodesSkipped int) string {
	sizeMB := float64(sizeBytes) / (1024 * 1024)
	durationStr := formatDuration(duration)
	return fmt.Sprintf("✅ *Scheduled Backup Completed*\nFile: `%s`\nSize: %.1f MB\nDuration: %s\nNodes: %d included, %d skipped",
		filename, sizeMB, durationStr, nodesIncluded, nodesSkipped)
}

// formatBackupFailure formats a Telegram notification message for a failed scheduled backup.
func formatBackupFailure(filename string, errMsg string) string {
	return fmt.Sprintf("❌ *Scheduled Backup Failed*\nFile: `%s`\nError: %s",
		filename, errMsg)
}

// formatDuration formats a duration into a human-readable string (e.g. "45s", "2m 34s", "1h 5m").
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*60
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) - h*60
	return fmt.Sprintf("%dh %dm", h, m)
}
