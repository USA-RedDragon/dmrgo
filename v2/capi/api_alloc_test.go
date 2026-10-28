package main

import (
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func allocs(t *testing.T, path func() (func() error, error), runs int) {
	t.Helper()
	allocsWant(t, path, runs, 0)
}

func allocsWant(t *testing.T, path func() (func() error, error), runs int, want float64) {
	t.Helper()
	f, err := path()
	if err != nil {
		t.Fatal(err)
	}
	for range runs {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}
	n := testing.AllocsPerRun(runs, func() {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	})
	if n != want {
		t.Errorf("%.1f allocations per call, want %.1f", n, want)
	}
}

func TestAllocsAPIPDU(t *testing.T) {
	allocs(t, pduPath, 100)
}

func TestAllocsAPIDecodeAsGo(t *testing.T) {
	c, err := pdu.NewBSOutboundActivation(0x123456, 3191868)
	if err != nil {
		t.Fatal(err)
	}
	bits, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	goAllocs := testing.AllocsPerRun(100, func() { _ = pdu.DecodeCSBK(bits) })
	allocsWant(t, decodePath, 100, goAllocs)
}

func TestAllocsAPILayer2(t *testing.T) {
	allocs(t, layer2Path, 100)
}

func TestAllocsAPITX(t *testing.T) {
	allocs(t, txPath, 100)
}

func TestAllocsAPIVoice(t *testing.T) {
	b, err := os.ReadFile("../layer2/testdata/parrot_kerchunk.bin")
	if err != nil {
		t.Fatal(err)
	}
	allocs(t, func() (func() error, error) { return voicePath(b) }, 200)
}

func TestAllocsAPIRF(t *testing.T) {
	allocs(t, func() (func() error, error) { return rfPath(rfIQ9600(t)) }, 20)
}
