package noderegistry

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strings"
)

// ValidateNodeDomain resolves the domain's A records and checks if any match the node IP.
// Returns warnings if the domain doesn't point to the node IP, or an error if resolution fails.
// This is a non-blocking validation: it doesn't prevent the domain from being set.
func ValidateNodeDomain(domain, nodeIP string) (warnings []string, err error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("domain is empty")
	}

	nodeIP = strings.TrimSpace(nodeIP)
	if nodeIP == "" {
		return nil, fmt.Errorf("node IP is empty")
	}

	addrs, err := net.LookupHost(domain)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %q: %w", domain, err)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("no A records found for %q", domain)
	}

	for _, addr := range addrs {
		if addr == nodeIP {
			return nil, nil // Match found, no warnings
		}
	}

	// No match — return warning
	warnings = append(warnings, fmt.Sprintf(
		"domain %q resolves to [%s] but node IP is %s — IKEv2 certificate validation may fail",
		domain, strings.Join(addrs, ", "), nodeIP,
	))
	return warnings, nil
}

// GetDomain retrieves the domain field from knode_connections for a given node ID.
func (r *DBRegistry) GetDomain(ctx context.Context, id int64) (string, error) {
	var domain sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT domain FROM knode_connections WHERE id = $1`, id,
	).Scan(&domain)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNodeNotFound
		}
		return "", fmt.Errorf("get domain for node %d: %w", id, err)
	}
	if !domain.Valid {
		return "", nil
	}
	return domain.String, nil
}

// SetDomain updates the domain field in knode_connections for a given node ID.
func (r *DBRegistry) SetDomain(ctx context.Context, id int64, domain string) error {
	domain = strings.TrimSpace(domain)

	var domainVal any
	if domain == "" {
		domainVal = nil
	} else {
		domainVal = domain
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE knode_connections SET domain = $1, updated_at = NOW() WHERE id = $2`,
		domainVal, id,
	)
	if err != nil {
		return fmt.Errorf("set domain for node %d: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNodeNotFound
	}

	return nil
}

// GetNodeAddress retrieves the address (IP) of a knode_connections record.
func (r *DBRegistry) GetNodeAddress(ctx context.Context, id int64) (string, error) {
	var address string
	err := r.db.QueryRowContext(ctx,
		`SELECT address FROM knode_connections WHERE id = $1`, id,
	).Scan(&address)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNodeNotFound
		}
		return "", fmt.Errorf("get address for node %d: %w", id, err)
	}
	return address, nil
}
