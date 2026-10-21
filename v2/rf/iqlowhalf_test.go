package rf

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

func TestIQLowHalfAsmLayout(t *testing.T) {
	var r iqLowRx
	q := unsafe.Offsetof(r.q)
	got := [...]uintptr{
		unsafe.Offsetof(r.ar) - q, unsafe.Offsetof(r.ai) - q, unsafe.Offsetof(r.pow) - q,
		unsafe.Offsetof(r.step) - q, unsafe.Offsetof(r.n) - q, unsafe.Offsetof(r.buf) - q,
	}
	if want := [...]uintptr{4, 8, 12, 24, 34, 36}; got != want {
		t.Fatalf("offsets from q of ar, ai, pow, step, n, buf: got %v, want %v", got, want)
	}
	if !iqLowRecip || iqLowBuf != 16 || 2*iq0LowHz != 19200 {
		t.Fatal("the assembly of iqlowhalf_dsp.go takes the reciprocal path, 16 samples and 2*9600")
	}
}

func TestLeanPushAsmLayout(t *testing.T) {
	var r leanCore
	ns := unsafe.Offsetof(r.ns)
	got := [...]uintptr{unsafe.Offsetof(r.ring) - ns, unsafe.Offsetof(r.head) - ns, unsafe.Offsetof(r.ringSum) - ns, unsafe.Offsetof(r.signs) - ns}
	if want := [...]uintptr{8, 154, 192, 196}; got != want {
		t.Fatalf("offsets from ns of ring, head, ringSum, signs: got %v, want %v", got, want)
	}
	if leanRing != 146 || syncSymbols != 24 {
		t.Fatal("the assembly of iqlowhalf_dsp.go takes a ring of 146 and a sync of 24 symbols")
	}
}

func TestIQLowHalfAsmTable(t *testing.T) {
	src, err := os.ReadFile("iqlowhalf_dsp.go")
	if err != nil {
		t.Fatal(err)
	}
	var tab []uint16
	for _, m := range regexp.MustCompile(`(?m)^\t\.short (.*)$`).FindAllStringSubmatch(string(src), -1) {
		for _, f := range strings.Split(m[1], ",") {
			v, err := strconv.ParseUint(strings.TrimSpace(f), 10, 16)
			if err != nil {
				t.Fatal(err)
			}
			tab = append(tab, uint16(v))
		}
	}
	if len(tab) != len(iqLowTab) {
		t.Fatalf("table of the assembly has %d entries, want %d", len(tab), len(iqLowTab))
	}
	for i, v := range iqLowTab {
		if tab[i] != v {
			t.Fatalf("entry %d: assembly %d, iqLowTab %d", i, tab[i], v)
		}
	}
}
