package noderegistry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
)

// NodeRecord represents a node's connection credentials stored in the database.
type NodeRecord struct {
	ID            int64
	Name          string
	Address       string
	Port          int
	APIKeyEnc     []byte // AES-GCM encrypted (stored form)
	ClientCertPEM []byte // PEM-encoded client certificate
	ClientKeyEnc  []byte // AES-GCM encrypted (stored form) or plaintext PEM before Create
	CACertPEM     []byte // PEM-encoded CA certificate
	Enabled       bool
	Status        string
	LastSeenAt    sql.NullTime
	OwnerWorker   sql.NullString
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NodeInput holds the plaintext credentials for creating or updating a node record.
// The API key and client key are in plaintext and will be encrypted before storage.
type NodeInput struct {
	Name          string
	Address       string
	Port          int
	APIKey        []byte // plaintext — will be encrypted
	ClientCertPEM []byte // PEM
	ClientKeyPEM  []byte // plaintext PEM — will be encrypted
	CACertPEM     []byte // PEM
	Enabled       bool
}

// Registry manages node records in the database.
type Registry interface {
	Create(ctx context.Context, input *NodeInput) (int64, error)
	Update(ctx context.Context, id int64, input *NodeInput) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*NodeRecord, error)
	ListEnabled(ctx context.Context) ([]*NodeRecord, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
}

// DBRegistry is a database-backed implementation of Registry.
type DBRegistry struct {
	db        *sql.DB
	encryptor *Encryptor
	validator Validator
}

// NewDBRegistry creates a new database-backed registry.
func NewDBRegistry(db *sql.DB, encryptor *Encryptor) *DBRegistry {
	return &DBRegistry{
		db:        db,
		encryptor: encryptor,
		validator: Validator{},
	}
}

// Create validates and persists a new node record. The API key and client key
// are encrypted before storage. Returns the new record ID.
func (r *DBRegistry) Create(ctx context.Context, input *NodeInput) (int64, error) {
	// Build a temporary NodeRecord for validation (PEM fields validated as plaintext)
	record := &NodeRecord{
		Name:          input.Name,
		Address:       input.Address,
		Port:          input.Port,
		ClientCertPEM: input.ClientCertPEM,
		ClientKeyEnc:  input.ClientKeyPEM, // validator checks PEM validity on the plaintext
		CACertPEM:     input.CACertPEM,
	}
	if err := r.validator.Validate(record); err != nil {
		return 0, fmt.Errorf("validation failed: %w", err)
	}
	if len(input.APIKey) == 0 {
		return 0, ErrEmptyAPIKey
	}

	// Encrypt sensitive fields
	apiKeyEnc, err := r.encryptor.Encrypt(input.APIKey)
	if err != nil {
		return 0, fmt.Errorf("encrypt api key: %w", err)
	}
	clientKeyEnc, err := r.encryptor.Encrypt(input.ClientKeyPEM)
	if err != nil {
		return 0, fmt.Errorf("encrypt client key: %w", err)
	}

	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO knode_connections (name, address, grpc_port, api_key_enc, client_cert, client_key_enc, ca_cert, enabled, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'offline', ?, ?)`,
		input.Name, input.Address, input.Port, apiKeyEnc, input.ClientCertPEM, clientKeyEnc, input.CACertPEM, input.Enabled, now, now,
	)
	if err != nil {
		log.Printf("[noderegistry] Create failed for %q: %v", input.Name, err)
		return 0, fmt.Errorf("insert node: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}

	log.Printf("[noderegistry] Created node %q (id=%d) at %s:%d", input.Name, id, input.Address, input.Port)
	return id, nil
}

// Update validates and updates an existing node record. Credentials are re-encrypted.
func (r *DBRegistry) Update(ctx context.Context, id int64, input *NodeInput) error {
	record := &NodeRecord{
		Name:          input.Name,
		Address:       input.Address,
		Port:          input.Port,
		ClientCertPEM: input.ClientCertPEM,
		ClientKeyEnc:  input.ClientKeyPEM,
		CACertPEM:     input.CACertPEM,
	}
	if err := r.validator.Validate(record); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	if len(input.APIKey) == 0 {
		return ErrEmptyAPIKey
	}

	apiKeyEnc, err := r.encryptor.Encrypt(input.APIKey)
	if err != nil {
		return fmt.Errorf("encrypt api key: %w", err)
	}
	clientKeyEnc, err := r.encryptor.Encrypt(input.ClientKeyPEM)
	if err != nil {
		return fmt.Errorf("encrypt client key: %w", err)
	}

	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE knode_connections SET name = ?, address = ?, grpc_port = ?, api_key_enc = ?, client_cert = ?, client_key_enc = ?, ca_cert = ?, enabled = ?, updated_at = ?
		 WHERE id = ?`,
		input.Name, input.Address, input.Port, apiKeyEnc, input.ClientCertPEM, clientKeyEnc, input.CACertPEM, input.Enabled, now, id,
	)
	if err != nil {
		log.Printf("[noderegistry] Update failed for id=%d: %v", id, err)
		return fmt.Errorf("update node: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("node id=%d: %w", id, ErrNodeNotFound)
	}

	log.Printf("[noderegistry] Updated node id=%d (%q)", id, input.Name)
	return nil
}

// Delete removes a node record and all associated credentials from the database.
func (r *DBRegistry) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM knode_connections WHERE id = ?`, id)
	if err != nil {
		log.Printf("[noderegistry] Delete failed for id=%d: %v", id, err)
		return fmt.Errorf("delete node: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("node id=%d: %w", id, ErrNodeNotFound)
	}

	log.Printf("[noderegistry] Deleted node id=%d", id)
	return nil
}

// Get retrieves a single node record by ID.
func (r *DBRegistry) Get(ctx context.Context, id int64) (*NodeRecord, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, address, grpc_port, api_key_enc, client_cert, client_key_enc, ca_cert, enabled, status, last_seen_at, owner_worker, created_at, updated_at
		 FROM knode_connections WHERE id = ?`, id)

	rec, err := scanNodeRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("get node id=%d: %w", id, err)
	}
	return rec, nil
}

// ListEnabled returns all node records where enabled = true.
func (r *DBRegistry) ListEnabled(ctx context.Context) ([]*NodeRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, address, grpc_port, api_key_enc, client_cert, client_key_enc, ca_cert, enabled, status, last_seen_at, owner_worker, created_at, updated_at
		 FROM knode_connections WHERE enabled = ?`, true)
	if err != nil {
		return nil, fmt.Errorf("list enabled nodes: %w", err)
	}
	defer rows.Close()

	var records []*NodeRecord
	for rows.Next() {
		rec, err := scanNodeRecordRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return records, nil
}

// UpdateStatus sets the status field (online, offline, stale) and last_seen_at for a node.
func (r *DBRegistry) UpdateStatus(ctx context.Context, id int64, status string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE knode_connections SET status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
		status, now, now, id)
	if err != nil {
		return fmt.Errorf("update status for id=%d: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("node id=%d: %w", id, ErrNodeNotFound)
	}
	return nil
}

// DecryptAPIKey decrypts the stored API key for a NodeRecord.
func (r *DBRegistry) DecryptAPIKey(rec *NodeRecord) ([]byte, error) {
	return r.encryptor.Decrypt(rec.APIKeyEnc)
}

// DecryptClientKey decrypts the stored client private key for a NodeRecord.
func (r *DBRegistry) DecryptClientKey(rec *NodeRecord) ([]byte, error) {
	return r.encryptor.Decrypt(rec.ClientKeyEnc)
}

// ErrNodeNotFound indicates the requested node does not exist.
var ErrNodeNotFound = errors.New("node not found")

// scanNodeRecord scans a single row into a NodeRecord.
func scanNodeRecord(row *sql.Row) (*NodeRecord, error) {
	rec := &NodeRecord{}
	err := row.Scan(
		&rec.ID, &rec.Name, &rec.Address, &rec.Port,
		&rec.APIKeyEnc, &rec.ClientCertPEM, &rec.ClientKeyEnc, &rec.CACertPEM,
		&rec.Enabled, &rec.Status, &rec.LastSeenAt, &rec.OwnerWorker,
		&rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// scanNodeRecordRows scans a rows result into a NodeRecord.
func scanNodeRecordRows(rows *sql.Rows) (*NodeRecord, error) {
	rec := &NodeRecord{}
	err := rows.Scan(
		&rec.ID, &rec.Name, &rec.Address, &rec.Port,
		&rec.APIKeyEnc, &rec.ClientCertPEM, &rec.ClientKeyEnc, &rec.CACertPEM,
		&rec.Enabled, &rec.Status, &rec.LastSeenAt, &rec.OwnerWorker,
		&rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return rec, nil
}
