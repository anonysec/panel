package email

import "testing"

func TestSanitizeHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string unchanged",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "removes carriage return",
			input:    "Hello\rWorld",
			expected: "HelloWorld",
		},
		{
			name:     "removes newline",
			input:    "Hello\nWorld",
			expected: "HelloWorld",
		},
		{
			name:     "removes CRLF sequence",
			input:    "Subject\r\nBcc: attacker@evil.com",
			expected: "SubjectBcc: attacker@evil.com",
		},
		{
			name:     "removes multiple CRLF injections",
			input:    "Test\r\nBcc: a@b.com\r\nX-Injected: yes",
			expected: "TestBcc: a@b.comX-Injected: yes",
		},
		{
			name:     "empty string stays empty",
			input:    "",
			expected: "",
		},
		{
			name:     "only CRLF characters",
			input:    "\r\n\r\n",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHeader(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeHeader(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
