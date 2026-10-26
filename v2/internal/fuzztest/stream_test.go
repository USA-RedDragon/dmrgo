package fuzztest_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
)

func TestStream(t *testing.T) {
	t.Parallel()
	pool := [][33]byte{{0x11}, {0x22}}
	got := fuzztest.Stream(pool, []byte{0x01, 0xAA, 0xBB, 0x40, 0x02, 0xCC, 0x81, 0x01, 0xC1}, []byte{0x00, 0x00})
	want := append([]byte{0x2A, 0xBB, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC}, pool[1][:]...)
	if !bytes.Equal(got, want) {
		t.Fatalf("Stream = % x, want % x", got, want)
	}
	if got := fuzztest.Stream(nil, []byte{0x7F, 0xFF, 0x00}, nil); len(got) != 4096 {
		t.Fatalf("Stream gave %d octets, want 4096", len(got))
	}
}
