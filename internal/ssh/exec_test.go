package ssh

import (
	"strings"
	"testing"
)

func TestLimitedBufferCapsOutput(t *testing.T) {
	buffer := newLimitedBuffer(5)
	input := "0123456789"
	n, err := buffer.Write([]byte(input))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(input) {
		t.Fatalf("Write returned %d, want %d", n, len(input))
	}
	if got := buffer.String(); got != "01234" {
		t.Fatalf("buffer = %q", got)
	}
	if !buffer.Truncated() {
		t.Fatal("buffer should report truncation")
	}
}

func TestLimitedBufferAcceptsSubsequentWritesAfterLimit(t *testing.T) {
	buffer := newLimitedBuffer(3)
	for _, chunk := range []string{"ab", "cd", strings.Repeat("x", 8)} {
		if n, err := buffer.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if got := buffer.String(); got != "abc" {
		t.Fatalf("buffer = %q", got)
	}
}
