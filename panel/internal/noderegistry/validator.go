package noderegistry

import (
	"encoding/pem"
	"errors"
	"fmt"
)

// Validation errors returned by Validator.
var (
	ErrEmptyAddress    = errors.New("address must not be empty")
	ErrInvalidPort     = errors.New("port must be between 1 and 65535")
	ErrInvalidPEM      = errors.New("invalid PEM data: no valid PEM block found")
	ErrEmptyName       = errors.New("name must not be empty")
	ErrEmptyAPIKey     = errors.New("api key must not be empty")
	ErrEmptyClientCert = errors.New("client certificate must not be empty")
	ErrEmptyClientKey  = errors.New("client key must not be empty")
	ErrEmptyCACert     = errors.New("CA certificate must not be empty")
)

// Validator validates NodeRecord fields before persistence.
type Validator struct{}

// Validate checks that a NodeRecord has valid fields.
// Returns the first validation error encountered.
func (v Validator) Validate(r *NodeRecord) error {
	if r.Name == "" {
		return ErrEmptyName
	}
	if r.Address == "" {
		return ErrEmptyAddress
	}
	if r.Port < 1 || r.Port > 65535 {
		return ErrInvalidPort
	}
	if len(r.ClientCertPEM) == 0 {
		return ErrEmptyClientCert
	}
	if err := validatePEM(r.ClientCertPEM, "client certificate"); err != nil {
		return err
	}
	if len(r.ClientKeyEnc) == 0 {
		return ErrEmptyClientKey
	}
	if err := validatePEM(r.ClientKeyEnc, "client key"); err != nil {
		return err
	}
	if len(r.CACertPEM) == 0 {
		return ErrEmptyCACert
	}
	if err := validatePEM(r.CACertPEM, "CA certificate"); err != nil {
		return err
	}
	return nil
}

// validatePEM checks that data contains at least one valid PEM block.
func validatePEM(data []byte, field string) error {
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("%s: %w", field, ErrInvalidPEM)
	}
	return nil
}
