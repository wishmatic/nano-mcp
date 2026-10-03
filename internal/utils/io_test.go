package utils

import (
	"strings"
	"testing"
)

func TestReadLimited(t *testing.T) {
	if got := ReadLimited(strings.NewReader("  hello  ")); got != "hello" {
		t.Errorf("ReadLimited() = %q, want %q", got, "hello")
	}
}

func TestReadLimitedTruncates(t *testing.T) {
	got := ReadLimited(strings.NewReader(strings.Repeat("a", maxReadSize+100)))
	if len(got) != maxReadSize {
		t.Errorf("ReadLimited() length = %d, want %d", len(got), maxReadSize)
	}
}

func TestReadLimitedNil(t *testing.T) {
	if got := ReadLimited(nil); got != "" {
		t.Errorf("ReadLimited(nil) = %q, want empty", got)
	}
}
