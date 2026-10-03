package utils

import "testing"

func TestIsHTTP(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"http://example.com", true},
		{"https://example.com/path", true},
		{"//example.com", false},
		{"/relative", false},
		{"ftp://example.com", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsHTTP(tt.in); got != tt.want {
			t.Errorf("IsHTTP(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
