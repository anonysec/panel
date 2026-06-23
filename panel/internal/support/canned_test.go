//go:build !lite

package support

import "testing"

func TestSubstitutePlaceholders(t *testing.T) {
	tests := []struct {
		name string
		body string
		vars map[string]string
		want string
	}{
		{
			name: "single placeholder replaced",
			body: "Hello {{customer_name}}, welcome!",
			vars: map[string]string{"customer_name": "Alice"},
			want: "Hello Alice, welcome!",
		},
		{
			name: "multiple placeholders replaced",
			body: "Hi {{customer_name}}, your plan is {{plan_name}}.",
			vars: map[string]string{"customer_name": "Bob", "plan_name": "Premium"},
			want: "Hi Bob, your plan is Premium.",
		},
		{
			name: "unknown placeholder preserved",
			body: "Hi {{customer_name}}, expires {{unknown_key}}.",
			vars: map[string]string{"customer_name": "Eve"},
			want: "Hi Eve, expires {{unknown_key}}.",
		},
		{
			name: "whitespace inside braces tolerated",
			body: "Hello {{ customer_name }}, plan: {{  plan_name  }}.",
			vars: map[string]string{"customer_name": "Carol", "plan_name": "Basic"},
			want: "Hello Carol, plan: Basic.",
		},
		{
			name: "empty vars map leaves all placeholders",
			body: "Hi {{customer_name}}, ticket #{{ticket_id}}.",
			vars: map[string]string{},
			want: "Hi {{customer_name}}, ticket #{{ticket_id}}.",
		},
		{
			name: "nil vars map leaves all placeholders",
			body: "Dear {{customer_name}}",
			vars: nil,
			want: "Dear {{customer_name}}",
		},
		{
			name: "no placeholders returns body unchanged",
			body: "Plain text without any placeholders.",
			vars: map[string]string{"customer_name": "Alice"},
			want: "Plain text without any placeholders.",
		},
		{
			name: "empty body returns empty string",
			body: "",
			vars: map[string]string{"customer_name": "Alice"},
			want: "",
		},
		{
			name: "placeholder value can be empty string",
			body: "Name: {{customer_name}}.",
			vars: map[string]string{"customer_name": ""},
			want: "Name: .",
		},
		{
			name: "same placeholder used multiple times",
			body: "{{username}} logged in. Welcome {{username}}!",
			vars: map[string]string{"username": "admin"},
			want: "admin logged in. Welcome admin!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubstitutePlaceholders(tt.body, tt.vars)
			if got != tt.want {
				t.Errorf("SubstitutePlaceholders() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultPlaceholders(t *testing.T) {
	placeholders := DefaultPlaceholders()

	if len(placeholders) == 0 {
		t.Fatal("DefaultPlaceholders() returned empty slice")
	}

	expected := map[string]bool{
		"customer_name": true,
		"plan_name":     true,
		"expiry_date":   true,
		"username":      true,
		"node_name":     true,
		"ticket_id":     true,
	}

	for _, p := range placeholders {
		if !expected[p] {
			t.Errorf("unexpected placeholder %q in DefaultPlaceholders()", p)
		}
		delete(expected, p)
	}

	for missing := range expected {
		t.Errorf("missing expected placeholder %q from DefaultPlaceholders()", missing)
	}
}
