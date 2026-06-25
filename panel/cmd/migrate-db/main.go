// Command migrate-db copies data from an existing MariaDB/MySQL database
// to a PostgreSQL/TimescaleDB database. This is a one-time migration tool
// for transitioning KorisPanel from MariaDB to PostgreSQL.
//
// Usage:
//
//	go run ./panel/cmd/migrate-db \
//	  --source-dsn "user:pass@tcp(host:3306)/dbname" \
//	  --target-dsn "postgres://user:pass@host:5432/dbname?sslmode=disable"
//
// Or with environment variables:
//
//	PANEL_MARIADB_DSN="user:pass@tcp(host:3306)/dbname" \
//	PANEL_PG_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable" \
//	go run ./panel/cmd/migrate-db
//
// Use --dry-run to show what would be migrated without writing to PostgreSQL.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgxTx is the interface satisfied by pgx.Tx for executing statements.
type pgxTx interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

const batchSize = 500

func main() {
	// Support both --source-dsn / --target-dsn and shorter -source / -target flags.
	// Environment variable fallback order: --flag > PANEL_MARIADB_DSN/PANEL_PG_DSN > MIGRATE_SOURCE_DSN/MIGRATE_TARGET_DSN
	sourceDefault := os.Getenv("PANEL_MARIADB_DSN")
	if sourceDefault == "" {
		sourceDefault = os.Getenv("MIGRATE_SOURCE_DSN")
	}
	targetDefault := os.Getenv("PANEL_PG_DSN")
	if targetDefault == "" {
		targetDefault = os.Getenv("MIGRATE_TARGET_DSN")
	}

	sourceDSN := flag.String("source-dsn", sourceDefault, "MariaDB source DSN (or set PANEL_MARIADB_DSN)")
	targetDSN := flag.String("target-dsn", targetDefault, "PostgreSQL target DSN (or set PANEL_PG_DSN)")
	dryRun := flag.Bool("dry-run", false, "Show what would be migrated without writing to PostgreSQL")

	// Register short aliases
	flag.StringVar(sourceDSN, "source", sourceDefault, "Alias for --source-dsn")
	flag.StringVar(targetDSN, "target", targetDefault, "Alias for --target-dsn")

	flag.Parse()

	if *sourceDSN == "" || *targetDSN == "" {
		fmt.Fprintln(os.Stderr, "Usage: migrate-db --source-dsn <mariadb-dsn> --target-dsn <postgres-dsn> [--dry-run]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Environment variables:")
		fmt.Fprintln(os.Stderr, "  PANEL_MARIADB_DSN   MariaDB connection string")
		fmt.Fprintln(os.Stderr, "  PANEL_PG_DSN        PostgreSQL connection string")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fmt.Fprintln(os.Stderr, "  --source-dsn, -source   MariaDB source DSN")
		fmt.Fprintln(os.Stderr, "  --target-dsn, -target   PostgreSQL target DSN")
		fmt.Fprintln(os.Stderr, "  --dry-run               Show migration plan without writing data")
		os.Exit(1)
	}

	if *dryRun {
		log.Println("[migrate-db] DRY RUN mode — no data will be written to PostgreSQL")
	}

	ctx := context.Background()

	// Connect to MariaDB source
	log.Println("[migrate-db] Connecting to MariaDB source...")
	srcDB, err := sql.Open("mysql", *sourceDSN+"?parseTime=true&charset=utf8mb4")
	if err != nil {
		log.Fatalf("[migrate-db] Failed to open source DB: %v", err)
	}
	defer srcDB.Close()
	if err := srcDB.PingContext(ctx); err != nil {
		log.Fatalf("[migrate-db] Failed to ping source DB: %v", err)
	}
	log.Println("[migrate-db] Source connection OK")

	// Connect to PostgreSQL target (skip in dry-run mode)
	var pool *pgxpool.Pool
	if !*dryRun {
		log.Println("[migrate-db] Connecting to PostgreSQL target...")
		pool, err = pgxpool.New(ctx, *targetDSN)
		if err != nil {
			log.Fatalf("[migrate-db] Failed to open target DB: %v", err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			log.Fatalf("[migrate-db] Failed to ping target DB: %v", err)
		}
		log.Println("[migrate-db] Target connection OK")
	}

	// Run migration
	start := time.Now()
	tables := migrationTables()
	totalRows := 0
	migratedTables := 0
	skippedTables := 0
	failedTables := 0

	for _, tbl := range tables {
		if *dryRun {
			n, err := dryRunTable(ctx, srcDB, tbl)
			if err != nil {
				log.Printf("[migrate-db] SKIP %s: %v", tbl.name, err)
				skippedTables++
				continue
			}
			totalRows += n
			migratedTables++
			log.Printf("[migrate-db] [dry-run] %s: %d rows would be migrated", tbl.name, n)
		} else {
			n, err := migrateTable(ctx, srcDB, pool, tbl)
			if err != nil {
				log.Printf("[migrate-db] ERROR migrating %s: %v (rolled back)", tbl.name, err)
				failedTables++
				continue
			}
			totalRows += n
			migratedTables++
			if n > 0 {
				log.Printf("[migrate-db] ✓ %s: %d rows", tbl.name, n)
			} else {
				log.Printf("[migrate-db] ✓ %s: empty (schema created)", tbl.name)
			}
		}
	}

	elapsed := time.Since(start)
	log.Printf("[migrate-db] Done! Migrated %d rows across %d tables in %s", totalRows, migratedTables, elapsed.Round(time.Millisecond))
	if skippedTables > 0 {
		log.Printf("[migrate-db] Skipped %d tables (not found in source)", skippedTables)
	}
	if failedTables > 0 {
		log.Printf("[migrate-db] Failed %d tables (see errors above)", failedTables)
		os.Exit(1)
	}
}

// tableSpec describes how to migrate a single table.
type tableSpec struct {
	name       string
	createDDL  string
	columns    []string
	primaryKey string // for ON CONFLICT clause (upsert)
}

// dryRunTable counts rows in the source table without writing anything.
func dryRunTable(ctx context.Context, src *sql.DB, spec tableSpec) (int, error) {
	var count int
	if err := src.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+spec.name).Scan(&count); err != nil {
		if strings.Contains(err.Error(), "doesn't exist") {
			return 0, fmt.Errorf("table not in source")
		}
		return 0, fmt.Errorf("count source rows: %w", err)
	}
	return count, nil
}

func migrateTable(ctx context.Context, src *sql.DB, dst *pgxpool.Pool, spec tableSpec) (int, error) {
	// Create target table (DDL runs outside transaction since CREATE TABLE
	// with IF NOT EXISTS is idempotent and some DDL cannot run inside a tx)
	if _, err := dst.Exec(ctx, spec.createDDL); err != nil {
		return 0, fmt.Errorf("create table: %w", err)
	}

	// Count rows in source
	var count int
	if err := src.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+spec.name).Scan(&count); err != nil {
		// Table might not exist in source (newer tables)
		if strings.Contains(err.Error(), "doesn't exist") {
			log.Printf("[migrate-db] Skipping %s: table not in source", spec.name)
			return 0, nil
		}
		return 0, fmt.Errorf("count source rows: %w", err)
	}

	if count == 0 {
		return 0, nil
	}

	// Begin a transaction for data insertion — rollback on any error
	tx, err := dst.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint: no-op after commit

	// Migrate in batches within the transaction
	colList := strings.Join(spec.columns, ", ")
	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT ? OFFSET ?", colList, spec.name, spec.primaryKey)

	migrated := 0
	for offset := 0; offset < count; offset += batchSize {
		rows, err := src.QueryContext(ctx, query, batchSize, offset)
		if err != nil {
			return migrated, fmt.Errorf("query at offset %d: %w", offset, err)
		}

		batch, err := insertBatchTx(ctx, tx, spec, rows)
		rows.Close()
		if err != nil {
			return migrated, fmt.Errorf("insert batch at offset %d: %w", offset, err)
		}
		migrated += batch

		// Log progress for large tables
		if count > batchSize && (offset+batchSize)%5000 == 0 {
			log.Printf("[migrate-db]   %s: %d/%d rows...", spec.name, migrated, count)
		}
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	// Reset sequence if table has a SERIAL primary key (outside tx, non-critical)
	if spec.primaryKey == "id" || spec.primaryKey == "radacctid" {
		resetSeq := fmt.Sprintf(
			"SELECT setval(pg_get_serial_sequence('%s', '%s'), COALESCE(MAX(%s), 1)) FROM %s",
			spec.name, spec.primaryKey, spec.primaryKey, spec.name,
		)
		if _, err := dst.Exec(ctx, resetSeq); err != nil {
			log.Printf("[migrate-db] Warning: could not reset sequence for %s: %v", spec.name, err)
		}
	}

	return migrated, nil
}

func insertBatchTx(ctx context.Context, tx pgxTx, spec tableSpec, rows *sql.Rows) (int, error) {
	cols := spec.columns
	numCols := len(cols)
	inserted := 0

	for rows.Next() {
		// Scan all columns as interface{}
		values := make([]interface{}, numCols)
		ptrs := make([]interface{}, numCols)
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return inserted, fmt.Errorf("scan: %w", err)
		}

		// Convert values for PostgreSQL compatibility
		pgValues := make([]interface{}, numCols)
		for i, v := range values {
			pgValues[i] = convertValue(v)
		}

		// Build INSERT statement with placeholders
		placeholders := make([]string, numCols)
		for i := range placeholders {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}

		var onConflict string
		if spec.primaryKey != "" {
			onConflict = fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", spec.primaryKey)
		}

		insertSQL := fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)%s",
			spec.name,
			strings.Join(cols, ", "),
			strings.Join(placeholders, ", "),
			onConflict,
		)

		if _, err := tx.Exec(ctx, insertSQL, pgValues...); err != nil {
			return inserted, fmt.Errorf("insert into %s: %w (values: %v)", spec.name, err, pgValues)
		}
		inserted++
	}

	return inserted, rows.Err()
}

// convertValue transforms MariaDB values into PostgreSQL-compatible types.
func convertValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []byte:
		// MySQL driver returns []byte for TEXT/VARCHAR columns
		return string(val)
	case time.Time:
		if val.IsZero() {
			return nil
		}
		return val
	case int64:
		return val
	case float64:
		return val
	case bool:
		return val
	default:
		return val
	}
}

// migrationTables returns all tables to migrate with their PostgreSQL DDL.
// Tables are ordered to respect foreign key dependencies.
func migrationTables() []tableSpec {
	return []tableSpec{
		tableAdmins(),
		tableAdminLoginAttempts(),
		tablePlans(),
		tableCustomers(),
		tableDiscountCodes(),
		tableSubscriptions(),
		tableWallets(),
		tableWalletTransactions(),
		tablePaymentMethods(),
		tablePayments(),
		tableTickets(),
		tableTicketMessages(),
		tableNodes(),
		tableNodeStatus(),
		tableNodeServices(),
		tableNodeUsageSnapshots(),
		tableNodeTasks(),
		tableVpnCoreSettings(),
		tableVpnProfiles(),
		tableWgPeers(),
		tableApiKeys(),
		tableApiLogs(),
		tableEvents(),
		tableAuditLogs(),
		tableDeletedArchive(),
		tableSettings(),
		tablePanelSettings(),
		tableBandwidthRules(),
		tableFirewallRules(),
		tableRadcheck(),
		tableRadacct(),
	}
}
