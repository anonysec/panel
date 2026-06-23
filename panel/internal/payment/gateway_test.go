//go:build !lite

package payment

import (
	"testing"
)

// mockGateway is a minimal Gateway implementation for testing.
type mockGateway struct {
	name string
}

func (m *mockGateway) Name() string { return m.name }
func (m *mockGateway) CreatePayment(amount float64, currency string, callbackURL string) (string, string, error) {
	return "https://example.com/pay", "ref123", nil
}
func (m *mockGateway) VerifyPayment(reference string) (float64, error)      { return 100.0, nil }
func (m *mockGateway) RefundPayment(reference string, amount float64) error { return nil }

func TestRegistry(t *testing.T) {
	t.Run("NewRegistry returns empty registry", func(t *testing.T) {
		r := NewRegistry()
		if names := r.List(); len(names) != 0 {
			t.Fatalf("expected empty list, got %v", names)
		}
	})

	t.Run("Register and Get", func(t *testing.T) {
		r := NewRegistry()
		g := &mockGateway{name: "test"}
		r.Register(g)

		got, ok := r.Get("test")
		if !ok {
			t.Fatal("expected to find registered gateway")
		}
		if got.Name() != "test" {
			t.Fatalf("expected name 'test', got %q", got.Name())
		}
	})

	t.Run("Get returns false for unknown gateway", func(t *testing.T) {
		r := NewRegistry()
		_, ok := r.Get("nonexistent")
		if ok {
			t.Fatal("expected false for unknown gateway")
		}
	})

	t.Run("List returns all registered names", func(t *testing.T) {
		r := NewRegistry()
		r.Register(&mockGateway{name: "alpha"})
		r.Register(&mockGateway{name: "beta"})

		names := r.List()
		if len(names) != 2 {
			t.Fatalf("expected 2 names, got %d", len(names))
		}

		found := map[string]bool{}
		for _, n := range names {
			found[n] = true
		}
		if !found["alpha"] || !found["beta"] {
			t.Fatalf("expected alpha and beta in list, got %v", names)
		}
	})

	t.Run("Register overwrites existing gateway", func(t *testing.T) {
		r := NewRegistry()
		r.Register(&mockGateway{name: "dup"})
		r.Register(&mockGateway{name: "dup"})

		names := r.List()
		if len(names) != 1 {
			t.Fatalf("expected 1 name after overwrite, got %d", len(names))
		}
	})
}

func TestZarinpal_Name(t *testing.T) {
	z := NewZarinpal("test-merchant-id", true)
	if z.Name() != "zarinpal" {
		t.Fatalf("expected 'zarinpal', got %q", z.Name())
	}
}

func TestZarinpal_RefundPayment(t *testing.T) {
	z := NewZarinpal("test-merchant-id", true)
	err := z.RefundPayment("some-ref", 50000)
	if err != nil {
		t.Fatalf("expected nil error from RefundPayment, got %v", err)
	}
}

func TestZarinpal_ImplementsGateway(t *testing.T) {
	// Compile-time check that Zarinpal implements Gateway.
	var _ Gateway = (*Zarinpal)(nil)
}
