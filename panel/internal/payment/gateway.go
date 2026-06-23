//go:build !lite

package payment

import "sync"

// Gateway defines the plugin interface for payment processors.
type Gateway interface {
	// Name returns the unique identifier for this gateway (e.g., "zarinpal", "stripe").
	Name() string
	// CreatePayment initiates a payment and returns a redirect URL and reference ID.
	CreatePayment(amount float64, currency string, callbackURL string) (redirectURL string, reference string, err error)
	// VerifyPayment confirms a payment by its reference. Returns the verified amount.
	VerifyPayment(reference string) (amount float64, err error)
	// RefundPayment issues a full or partial refund.
	RefundPayment(reference string, amount float64) error
}

// Registry holds all registered payment gateways.
type Registry struct {
	mu       sync.RWMutex
	gateways map[string]Gateway
}

// NewRegistry creates a new empty gateway registry.
func NewRegistry() *Registry {
	return &Registry{gateways: make(map[string]Gateway)}
}

// Register adds a gateway to the registry under its Name().
func (r *Registry) Register(g Gateway) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gateways[g.Name()] = g
}

// Get retrieves a gateway by name from the registry.
func (r *Registry) Get(name string) (Gateway, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.gateways[name]
	return g, ok
}

// List returns all registered gateway names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.gateways))
	for name := range r.gateways {
		names = append(names, name)
	}
	return names
}

// Deregister removes a gateway from the registry by name.
func (r *Registry) Deregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.gateways, name)
}
