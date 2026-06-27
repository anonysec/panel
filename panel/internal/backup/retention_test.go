package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApplyRetention_DeletesOldestBeyondLimit(t *testing.T) {
	// Setup temp directory with fake backup files
	tmpDir := t.TempDir()

	// Create fake archive files for 5 backups
	filenames := []string{
		"backup-2024-01-05-020000.tar.gz", // newest
		"backup-2024-01-04-020000.tar.gz",
		"backup-2024-01-03-020000.tar.gz",
		"backup-2024-01-02-020000.tar.gz", // should be deleted
		"backup-2024-01-01-020000.tar.gz", // should be deleted
	}
	for _, fn := range filenames {
		archivePath := filepath.Join(tmpDir, fn)
		if err := os.WriteFile(archivePath, []byte("archive-data"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archivePath+".sha256", []byte("checksum-data"), 0640); err != nil {
			t.Fatal(err)
		}
	}

	// Setup go-sqlmock
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Expect SELECT query returning 5 rows ordered by started_at DESC
	rows := sqlmock.NewRows([]string{"id", "filename"}).
		AddRow(5, filenames[0]).
		AddRow(4, filenames[1]).
		AddRow(3, filenames[2]).
		AddRow(2, filenames[3]).
		AddRow(1, filenames[4])

	mock.ExpectQuery(`SELECT id, filename FROM backups WHERE status='completed' ORDER BY started_at DESC`).
		WillReturnRows(rows)

	// Expect DELETE for the 2 oldest backups (ids 2 and 1)
	mock.ExpectExec(`DELETE FROM backups WHERE id=\$1`).WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM backups WHERE id=\$1`).WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Create service with RetentionCount=3
	svc := &Service{
		db: db,
		cfg: Config{
			StorageDir:     tmpDir,
			RetentionCount: 3,
		},
	}

	// Run retention
	svc.ApplyRetention(context.Background())

	// Assert: first 3 backups (newest) should still exist
	for i := 0; i < 3; i++ {
		archivePath := filepath.Join(tmpDir, filenames[i])
		if _, err := os.Stat(archivePath); os.IsNotExist(err) {
			t.Errorf("expected %s to still exist (within retention limit)", filenames[i])
		}
		if _, err := os.Stat(archivePath + ".sha256"); os.IsNotExist(err) {
			t.Errorf("expected %s.sha256 to still exist", filenames[i])
		}
	}

	// Assert: last 2 backups (oldest) should be deleted
	for i := 3; i < 5; i++ {
		archivePath := filepath.Join(tmpDir, filenames[i])
		if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
			t.Errorf("expected %s to be deleted (beyond retention limit)", filenames[i])
		}
		if _, err := os.Stat(archivePath + ".sha256"); !os.IsNotExist(err) {
			t.Errorf("expected %s.sha256 to be deleted", filenames[i])
		}
	}

	// Assert all DB expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestApplyRetention_NoDeletesAtOrBelowLimit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 3 fake backup files
	filenames := []string{
		"backup-2024-01-03-020000.tar.gz",
		"backup-2024-01-02-020000.tar.gz",
		"backup-2024-01-01-020000.tar.gz",
	}
	for _, fn := range filenames {
		archivePath := filepath.Join(tmpDir, fn)
		if err := os.WriteFile(archivePath, []byte("archive-data"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archivePath+".sha256", []byte("checksum-data"), 0640); err != nil {
			t.Fatal(err)
		}
	}

	// Setup go-sqlmock
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Expect SELECT query returning 3 rows (at limit)
	rows := sqlmock.NewRows([]string{"id", "filename"}).
		AddRow(3, filenames[0]).
		AddRow(2, filenames[1]).
		AddRow(1, filenames[2])

	mock.ExpectQuery(`SELECT id, filename FROM backups WHERE status='completed' ORDER BY started_at DESC`).
		WillReturnRows(rows)

	// No DELETE expected since we're at the retention limit

	// Create service with RetentionCount=3 (equal to number of backups)
	svc := &Service{
		db: db,
		cfg: Config{
			StorageDir:     tmpDir,
			RetentionCount: 3,
		},
	}

	// Run retention
	svc.ApplyRetention(context.Background())

	// Assert: all files should still exist
	for _, fn := range filenames {
		archivePath := filepath.Join(tmpDir, fn)
		if _, err := os.Stat(archivePath); os.IsNotExist(err) {
			t.Errorf("expected %s to still exist (at retention limit)", fn)
		}
		if _, err := os.Stat(archivePath + ".sha256"); os.IsNotExist(err) {
			t.Errorf("expected %s.sha256 to still exist", fn)
		}
	}

	// Assert all DB expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestApplyRetention_BelowLimit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 2 fake backup files (below retention limit of 5)
	filenames := []string{
		"backup-2024-01-02-020000.tar.gz",
		"backup-2024-01-01-020000.tar.gz",
	}
	for _, fn := range filenames {
		archivePath := filepath.Join(tmpDir, fn)
		if err := os.WriteFile(archivePath, []byte("archive-data"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archivePath+".sha256", []byte("checksum-data"), 0640); err != nil {
			t.Fatal(err)
		}
	}

	// Setup go-sqlmock
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "filename"}).
		AddRow(2, filenames[0]).
		AddRow(1, filenames[1])

	mock.ExpectQuery(`SELECT id, filename FROM backups WHERE status='completed' ORDER BY started_at DESC`).
		WillReturnRows(rows)

	// No DELETE expected since we're below the retention limit

	svc := &Service{
		db: db,
		cfg: Config{
			StorageDir:     tmpDir,
			RetentionCount: 5,
		},
	}

	svc.ApplyRetention(context.Background())

	// Assert: all files should still exist
	for _, fn := range filenames {
		archivePath := filepath.Join(tmpDir, fn)
		if _, err := os.Stat(archivePath); os.IsNotExist(err) {
			t.Errorf("expected %s to still exist (below retention limit)", fn)
		}
		if _, err := os.Stat(archivePath + ".sha256"); os.IsNotExist(err) {
			t.Errorf("expected %s.sha256 to still exist", fn)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
