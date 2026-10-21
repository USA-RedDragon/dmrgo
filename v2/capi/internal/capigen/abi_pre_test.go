package capigen

import (
	"strings"
	"testing"
)

func TestLockPrerelease(t *testing.T) {
	t.Parallel()

	syms := []string{"sym #define B 2"}
	for _, pre := range []bool{true, false} {
		in := "abi 1\nsym #define A 1\n"
		if pre {
			in = "abi 1\nprerelease\nsym #define A 1\n"
		}
		lock, err := parseLock([]byte(in))
		if err != nil {
			t.Fatal(err)
		}
		out, broken, err := lock.render(syms, false)
		if len(broken) != 1 {
			t.Fatalf("prerelease %v: broken %v", pre, broken)
		}
		if !pre {
			if err == nil {
				t.Error("a change without the prerelease marker passed without -break")
			}
			continue
		}
		if err != nil {
			t.Fatalf("prerelease: %v", err)
		}
		if want := "abi 1\nprerelease\nsym #define B 2\n"; string(out) != want {
			t.Errorf("prerelease: lock %q, want %q", out, want)
		}
	}
	lock, err := parseLock([]byte("abi 1\nsym #define A 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := lock.render(syms, true)
	if err != nil || !strings.HasPrefix(string(out), "abi 2\n") {
		t.Errorf("-break without the marker: %q, %v; want abi 2", out, err)
	}
}
