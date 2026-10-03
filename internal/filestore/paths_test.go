package filestore

import (
	"path/filepath"
	"testing"
)

func TestSafePathRejectsKeysThatEscape(t *testing.T) {
	client, _ := testClient(t)

	tests := map[string]string{
		"empty":                    "",
		"absolute":                 "/v/2026-10/a.mp4",
		"outside the namespace":    "i/2026-10/a.mp4",
		"backing out":              "v/../a.mp4",
		"a directory of its own":   "v/./a.mp4",
		"a trailing empty segment": "v/a.mp4/",
	}

	for name, key := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := client.safePath(key); err == nil {
				t.Errorf("safePath(%q) error = nil, want a rejection", key)
			}
		})
	}
}

func TestSafePathKeepsKeysUnderTheStore(t *testing.T) {
	client, dir := testClient(t)

	path, err := client.safePath("v/2026-10/a.mp4")
	if err != nil {
		t.Fatalf("safePath() error: %v", err)
	}

	if want := filepath.Join(dir, "v", "2026-10", "a.mp4"); path != want {
		t.Errorf("safePath() = %q, want %q", path, want)
	}
}
