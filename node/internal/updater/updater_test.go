package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyChecksum(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
		want     bool
	}{
		{
			name:     "valid checksum matches",
			data:     []byte("hello world"),
			expected: sha256HexString([]byte("hello world")),
			want:     true,
		},
		{
			name:     "mismatched checksum",
			data:     []byte("hello world"),
			expected: sha256HexString([]byte("different data")),
			want:     false,
		},
		{
			name:     "empty data with correct checksum",
			data:     []byte{},
			expected: sha256HexString([]byte{}),
			want:     true,
		},
		{
			name:     "uppercase expected hex still matches",
			data:     []byte("test binary data"),
			expected: upperHex(sha256HexString([]byte("test binary data"))),
			want:     true,
		},
		{
			name:     "expected with leading/trailing whitespace",
			data:     []byte("trimme"),
			expected: "  " + sha256HexString([]byte("trimme")) + "  ",
			want:     true,
		},
		{
			name:     "completely wrong checksum",
			data:     []byte("some data"),
			expected: "0000000000000000000000000000000000000000000000000000000000000000",
			want:     false,
		},
		{
			name:     "empty expected string",
			data:     []byte("data"),
			expected: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyChecksum(tt.data, tt.expected)
			if got != tt.want {
				t.Errorf("VerifyChecksum() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		remote  string
		want    bool
	}{
		{
			name:    "remote patch is higher",
			current: "1.0.0",
			remote:  "1.0.1",
			want:    true,
		},
		{
			name:    "remote minor is higher",
			current: "1.0.5",
			remote:  "1.1.0",
			want:    true,
		},
		{
			name:    "remote major is higher",
			current: "1.9.9",
			remote:  "2.0.0",
			want:    true,
		},
		{
			name:    "versions are equal",
			current: "1.2.3",
			remote:  "1.2.3",
			want:    false,
		},
		{
			name:    "current is newer (patch)",
			current: "1.0.2",
			remote:  "1.0.1",
			want:    false,
		},
		{
			name:    "current is newer (minor)",
			current: "1.5.0",
			remote:  "1.4.9",
			want:    false,
		},
		{
			name:    "current is newer (major)",
			current: "3.0.0",
			remote:  "2.9.9",
			want:    false,
		},
		{
			name:    "with v prefix on both",
			current: "v1.0.0",
			remote:  "v1.0.1",
			want:    true,
		},
		{
			name:    "v prefix only on current",
			current: "v1.0.0",
			remote:  "1.0.1",
			want:    true,
		},
		{
			name:    "v prefix only on remote",
			current: "1.0.0",
			remote:  "v2.0.0",
			want:    true,
		},
		{
			name:    "equal versions with v prefix",
			current: "v1.2.3",
			remote:  "v1.2.3",
			want:    false,
		},
		{
			name:    "invalid current version",
			current: "invalid",
			remote:  "1.0.0",
			want:    false,
		},
		{
			name:    "invalid remote version",
			current: "1.0.0",
			remote:  "invalid",
			want:    false,
		},
		{
			name:    "both invalid",
			current: "abc",
			remote:  "xyz",
			want:    false,
		},
		{
			name:    "version with only two parts is invalid",
			current: "1.0",
			remote:  "1.0.1",
			want:    false,
		},
		{
			name:    "large version numbers",
			current: "10.20.30",
			remote:  "10.20.31",
			want:    true,
		},
		{
			name:    "whitespace around versions",
			current: " v1.0.0 ",
			remote:  " v1.0.1 ",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVersions(tt.current, tt.remote)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %v, want %v", tt.current, tt.remote, got, tt.want)
			}
		})
	}
}

// sha256HexString computes the SHA-256 hex string for the given data.
func sha256HexString(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// upperHex converts a hex string to uppercase.
func upperHex(s string) string {
	result := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'f' {
			c = c - 'a' + 'A'
		}
		result[i] = c
	}
	return string(result)
}
