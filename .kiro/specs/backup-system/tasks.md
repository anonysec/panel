# Implementation Plan: Backup System Upgrade

## Overview

This plan replaces the current minimal SQL dump goroutine in panel/cmd/panel/main.go with a full-featured backup/restore system. It covers: backup service package, database migration, SQL dump streaming, node config collection, archive packaging, checksum computation, scheduling, retention, restore from upload, admin API endpoints, admin UI views, and property-based tests.

## Tasks

- [ ] 1. Database migration and backup service foundation
  - [ ] 1.1 Create database migration for backups table
    - Create migration file `panel/migrations/XXX_backups.sql` (next sequential number)
    - CREATE TABLE backups with columns: id, filename, status (ENUM: in_progress/completed/failed), type (ENUM: manual/scheduled/pre_restore), size_bytes, checksum, nodes_included (JSON), nodes_skipped (JSON), error_message (TEXT), started_at, completed_at
    - Add INDEX on status and started_at columns
    - Add backup_schedule and backup_retention_count to panel_settings if not exists
    - _Requirements: 10.1, 6.3, 7.4_

  - [ ] 1.2 Create backup service package `panel/internal/backup/service.go`
    - Define Service struct with db, config, sync.Mutex, and scheduler fields
    - Implement New(db, cfg) constructor
    - Implement LoadConfigFromDB() to read schedule and retention from panel_settings
    - Implement UpdateConfig(schedule, retentionCount) to persist settings
    - Define BackupRecord struct matching database schema
    - Define Config struct with StorageDir, Schedule, RetentionCount, DBUser, DBPass, DBName
    - _Requirements: 6.3, 7.4, 11.2_

  - [ ] 1.3 Create backup naming and storage utilities `panel/internal/backup/storage.go`
    - Implement generateFilename(time.Time) returning "backup-{YYYY-MM-DD-HHmmss}.tar.gz"
    - Implement ensureStorageDir(path) to create /opt/KorisPanel/backups/ with 0750 permissions
    - Implement listArchiveFiles(dir) to scan storage directory for existing .tar.gz files
    - _Requirements: 3.2, 3.4, 3.5_

- [ ] 2. Core backup creation (SQL dump + archive packaging)
  - [ ] 2.1 Implement SQL dump streaming `panel/internal/backup/dump.go`
    - Implement streamMySQLDump(ctx, cfg) returning io.ReadCloser that streams mysqldump output
    - Use exec.CommandContext with --single-transaction --routines flags
    - Set MYSQL_PWD environment variable for credential passing
    - Capture stderr for error reporting on non-zero exit
    - _Requirements: 1.1, 1.2, 1.3, 1.5, 11.1_

  - [ ] 2.2 Implement archive writer `panel/internal/backup/archive.go`
    - Implement WriteArchive(outputPath, dumpReader, nodeConfigs, manifest) that creates .tar.gz
    - Stream dump.sql from reader directly into tar without full buffering
    - Write configs/{node_name}/... files from collected node data
    - Write manifest.json as the last entry
    - Use compress/gzip and archive/tar from standard library
    - _Requirements: 3.1, 3.3, 11.1, 11.3_

  - [ ] 2.3 Implement checksum computation `panel/internal/backup/checksum.go`
    - Implement ComputeChecksum(filePath) returning hex-encoded SHA-256 hash
    - Implement WriteChecksumFile(archivePath, checksum) writing {archivePath}.sha256
    - Implement VerifyChecksum(filePath, expectedHash) returning (bool, error)
    - Stream file through hash writer to avoid loading entire file in memory
    - _Requirements: 4.2, 4.3, 4.4, 4.5_

  - [ ] 2.4 Implement manifest generation `panel/internal/backup/manifest.go`
    - Define Manifest struct with Version, Timestamp, PanelVersion, Database, NodesIncluded, NodesSkipped, Files, ChecksumAlgorithm, Checksum
    - Implement GenerateManifest(timestamp, dbName, nodesIncluded, nodesSkipped, filesInfo) returning Manifest
    - Implement JSON serialization with proper ISO 8601 timestamp formatting
    - _Requirements: 4.1_

  - [ ] 2.5 Implement CreateBackup orchestrator in `panel/internal/backup/service.go`
    - Acquire mutex; return error if already locked (concurrent prevention)
    - Insert backup record with status "in_progress"
    - Execute streamMySQLDump, pipe into archive writer
    - Query online nodes, dispatch backup.collect_configs tasks
    - Wait up to 60s for node task completions (poll DB for task status)
    - Package everything via WriteArchive
    - Compute and store checksum
    - Update backup record to "completed" with size, checksum, nodes info
    - On any failure, update record to "failed" with error message
    - Apply retention policy after successful completion
    - _Requirements: 1.1, 1.4, 2.1, 2.5, 2.6, 3.1, 4.2, 6.4, 10.2, 10.3, 10.4, 11.2_

- [ ] 3. Node agent config collection and restore handlers
  - [ ] 3.1 Implement backup.collect_configs task handler in `node/cmd/node/main.go`
    - Collect files from /etc/openvpn/, /etc/wireguard/, /etc/ipsec.d/, /etc/xl2tpd/
    - Skip non-existent directories gracefully
    - Create tar archive of collected files preserving directory structure
    - Base64-encode the tar and return in task completion result
    - Include files_count and total_size in response
    - Limit total collected data to 10MB to prevent memory issues
    - _Requirements: 2.2, 2.3, 11.4_

  - [ ] 3.2 Implement backup.restore_configs task handler in `node/cmd/node/main.go`
    - Accept base64-encoded tar from task payload
    - Decode and extract files to their original absolute paths
    - Restart affected services: openvpn, wg-quick@wg0, ipsec, xl2tpd (only if config files for that service were restored)
    - Report success/failure in task completion
    - _Requirements: 8.4_

- [ ] 4. Scheduling and retention
  - [ ] 4.1 Implement backup scheduler `panel/internal/backup/schedule.go`
    - Implement ParseSchedule(value string) returning Schedule struct (type: daily/weekly/disabled, hour, weekday)
    - Implement ShouldRun(schedule, currentTime) bool — evaluates whether the current minute matches
    - Implement StartScheduler() that ticks every minute and calls CreateBackup when ShouldRun is true
    - Prevent triggering multiple times for the same scheduled slot (track last trigger time)
    - _Requirements: 6.1, 6.2, 6.4, 6.5_

  - [ ] 4.2 Implement retention policy `panel/internal/backup/retention.go`
    - Implement ApplyRetention() that queries completed backups ordered by started_at DESC
    - Skip first K records (retention count), delete remaining
    - Delete archive file + .sha256 companion file from disk
    - Mark deleted backup records (or leave them with file_deleted flag)
    - Do not delete backups with status "in_progress"
    - _Requirements: 7.1, 7.2, 7.3, 7.5_

  - [ ]* 4.3 Write property test for retention policy (Property 3)
    - **Property 3: Retention policy preserves newest backups**
    - Generate random lists of backup records with varying timestamps and retention counts K
    - Verify exactly max(0, N-K) deletions, all deleted records older than any retained record
    - File: `panel/internal/backup/retention_test.go`
    - **Validates: Requirements 7.2, 7.3**

  - [ ]* 4.4 Write property test for schedule matching (Property 4)
    - **Property 4: Schedule matching correctness**
    - Generate random timestamps and schedule configs; verify ShouldRun returns true iff time matches pattern
    - File: `panel/internal/backup/schedule_test.go`
    - **Validates: Requirements 6.2**

- [ ] 5. Checkpoint - Ensure all backend core tests pass
  - Run `go test ./panel/internal/backup/...` and verify all pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 6. Admin API endpoints
  - [ ] 6.1 Implement backup API handlers in `panel/internal/api/backup.go`
    - GET /api/admin/backups — list all backup records from DB
    - POST /api/admin/backups — trigger CreateBackup (async, return backup ID immediately)
    - GET /api/admin/backups/{id}/download — stream file with Content-Type: application/gzip, Content-Disposition: attachment
    - POST /api/admin/backups/{id}/verify — call VerifyIntegrity, return {valid: bool, details}
    - DELETE /api/admin/backups/{id} — call DeleteBackup
    - POST /api/admin/backups/restore — multipart file upload, call RestoreFromUpload
    - GET /api/admin/backups/settings — return current schedule and retention from panel_settings
    - PUT /api/admin/backups/settings — update schedule and retention in panel_settings
    - _Requirements: 5.1, 5.2, 5.3, 9.1, 9.2, 9.3, 9.4_

  - [ ] 6.2 Implement restore orchestration in `panel/internal/backup/restore.go`
    - Implement RestoreFromUpload(ctx, file, filename) error
    - Validate uploaded file: must be .tar.gz, must contain dump.sql and manifest.json
    - Parse manifest, verify checksum if present
    - Create pre-restore safety backup (type="pre_restore")
    - Apply dump.sql via `mysql` command with --force flag (streaming from tar reader)
    - Extract configs per node, dispatch backup.restore_configs tasks
    - Track restore progress in backup record
    - _Requirements: 8.1, 8.2, 8.3, 8.5, 8.6, 8.7_

  - [ ]* 6.3 Write property test for archive structure round-trip (Property 1)
    - **Property 1: Archive structure round-trip**
    - Generate random file trees and dump content; create archive then extract and verify structure matches manifest
    - File: `panel/internal/backup/archive_test.go`
    - **Validates: Requirements 3.3**

  - [ ]* 6.4 Write property test for checksum integrity (Property 2)
    - **Property 2: Checksum integrity verification**
    - Generate random byte content, write to temp file, compute checksum, verify VerifyChecksum returns true
    - Mutate one byte, verify VerifyChecksum returns false
    - File: `panel/internal/backup/checksum_test.go`
    - **Validates: Requirements 4.2, 4.3, 4.4**

  - [ ]* 6.5 Write property test for restore validation (Property 8)
    - **Property 8: Restore validation rejects invalid archives**
    - Generate random invalid tar.gz files (missing dump.sql, missing manifest, corrupt tar, non-gzip)
    - Verify RestoreFromUpload returns validation error without DB modification
    - File: `panel/internal/backup/restore_test.go`
    - **Validates: Requirements 8.1, 8.5**

  - [ ]* 6.6 Write property test for manifest completeness (Property 5)
    - **Property 5: Manifest generation completeness**
    - Generate random sets of online/offline nodes; verify manifest includes all nodes partitioned correctly
    - File: `panel/internal/backup/manifest_test.go`
    - **Validates: Requirements 4.1, 2.5, 2.6**

  - [ ]* 6.7 Write property test for filename uniqueness (Property 6)
    - **Property 6: Backup filename uniqueness**
    - Generate pairs of distinct timestamps; verify generated filenames are never equal
    - File: `panel/internal/backup/naming_test.go`
    - **Validates: Requirements 3.2**

- [ ] 7. Integration into main.go and removal of old backup code
  - [ ] 7.1 Remove old backup goroutine from panel/cmd/panel/main.go
    - Remove the `if t.Hour() == 2 && t.Minute() == 0` block from startWorker
    - Keep mysqlCredsFromDSN function (used by backup service)
    - _Requirements: 1.1_

  - [ ] 7.2 Initialize backup service in main.go
    - Create backup.Service in main() after database initialization
    - Load config from panel_settings (with fallback defaults: daily:02, retention:7)
    - Call service.StartScheduler() to begin automatic backup scheduling
    - _Requirements: 6.2, 6.3_

  - [ ] 7.3 Register backup API routes
    - Add backup handler routes to the mux in main.go (or in api.Routes())
    - Ensure admin auth middleware protects all backup endpoints
    - _Requirements: 5.1, 9.1_

  - [ ] 7.4 Update Telegram bot /backup command
    - Change bot.cmdBackup to mention "Settings → Backup" page for full management
    - Optionally trigger a backup and report status back to chat
    - _Requirements: (existing bot integration improvement)_

- [ ] 8. Checkpoint - Ensure all backend tests pass
  - Run `go test ./...` and verify all pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 9. Frontend admin — Backup management views
  - [ ] 9.1 Create backup API composable `panel/web/admin/src/composables/useBackups.ts`
    - Functions: fetchBackups(), createBackup(), downloadBackup(id), verifyBackup(id), deleteBackup(id), restoreBackup(file), getSettings(), updateSettings(schedule, retention)
    - Handle async backup creation (poll for status updates)
    - _Requirements: 9.1, 9.2, 9.3, 9.4_

  - [ ] 9.2 Create backup management view `panel/web/admin/src/views/BackupView.vue`
    - Backup list table: filename, timestamp, size (formatted), status badge, nodes count, actions
    - Action buttons per row: download, verify, restore, delete
    - "Create Backup Now" button at top
    - Progress indicator when backup/restore is in_progress (poll status every 3s)
    - Confirmation dialog before restore with warning about data overwrite
    - _Requirements: 9.1, 9.2, 9.4, 9.5, 9.6_

  - [ ] 9.3 Create backup settings component `panel/web/admin/src/components/BackupSettings.vue`
    - Schedule selector: disabled / daily (hour picker) / weekly (day + hour picker)
    - Retention count input (number, min 1, max 30, default 7)
    - Save button persists via PUT /api/admin/backups/settings
    - Display next scheduled backup time
    - _Requirements: 6.1, 7.1, 9.3_

  - [ ] 9.4 Create restore dialog component `panel/web/admin/src/components/BackupRestoreDialog.vue`
    - File upload input accepting .tar.gz files
    - Display file name and size after selection
    - Warning message: "This will overwrite the current database. A safety backup will be created first."
    - Confirm/Cancel buttons
    - Progress state during restore
    - _Requirements: 8.1, 9.6_

  - [ ] 9.5 Register backup route in admin router
    - Add route `/settings/backup` or `/backups` to admin Vue Router
    - Add navigation link in settings sidebar/menu
    - _Requirements: 9.1_

- [ ] 10. Final checkpoint - Ensure all tests pass
  - Run `go test ./...` for backend
  - Run `npm run test` in panel/web/admin for frontend
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are property-based tests that can be skipped for faster MVP delivery
- The existing `mysqlCredsFromDSN` function in main.go is reused by the backup service
- Storage path /opt/KorisPanel/backups/ replaces the old /var/backups/KorisPanel/ path
- The backup service uses a sync.Mutex for concurrent prevention (simple, appropriate for single-process deployment)
- Node config collection has a 60-second timeout per node; nodes that timeout are still listed in the manifest as "skipped"
- The pre-restore safety backup ensures admins can always roll back a failed restore
- Property tests use `pgregory.net/rapid` for Go as specified in the tech stack
- Each task references specific requirements for traceability
- The backup.collect_configs task payload is empty since all config paths are hardcoded in the node agent

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2", "1.3"] },
    { "id": 1, "tasks": ["2.1", "2.2", "2.3", "2.4"] },
    { "id": 2, "tasks": ["2.5", "3.1", "3.2"] },
    { "id": 3, "tasks": ["4.1", "4.2", "4.3", "4.4"] },
    { "id": 4, "tasks": ["6.1", "6.2"] },
    { "id": 5, "tasks": ["6.3", "6.4", "6.5", "6.6", "6.7"] },
    { "id": 6, "tasks": ["7.1", "7.2", "7.3", "7.4"] },
    { "id": 7, "tasks": ["9.1", "9.2", "9.3", "9.4", "9.5"] }
  ]
}
```
