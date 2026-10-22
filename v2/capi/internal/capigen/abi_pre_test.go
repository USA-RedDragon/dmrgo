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


func TestLockRetired(t *testing.T) {
	t.Parallel()

	lock, err := parseLock([]byte("abi 1\ncode DMRGO_ERR_A -100\ncode DMRGO_ERR_B -101\n"))
	if err != nil {
		t.Fatal(err)
	}
	lock.code("DMRGO_ERR_A")
	out, _, err := lock.render(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := "abi 2\ncode DMRGO_ERR_A -100\nretired code DMRGO_ERR_B -101\n"; string(out) != want {
		t.Fatalf("lock %q, want %q", out, want)
	}
	lock, err = parseLock(out)
	if err != nil {
		t.Fatal(err)
	}
	lock.code("DMRGO_ERR_A")
	if c := lock.code("DMRGO_ERR_C"); c != -102 {
		t.Errorf("a new code after a retired -101 got %d, want -102", c)
	}
	lock.renumber([]string{"DMRGO_ERR_C"}, nil)
	if c := lock.code("DMRGO_ERR_C"); c != -102 {
		t.Errorf("renumber after the release moved a code to %d", c)
	}
}

func TestLockRenumber(t *testing.T) {
	t.Parallel()

	lock, err := parseLock([]byte("abi 1\nprerelease\ncode DMRGO_ERR_Z -100\ncode DMRGO_ERR_A -107\ntag u_t U_B 1\nretired code DMRGO_ERR_Q -101\n"))
	if err != nil {
		t.Fatal(err)
	}
	lock.renumber([]string{"DMRGO_ERR_A", "DMRGO_ERR_Z"}, map[string][]string{"u_t": {"U_A", "U_B"}})
	if a, z := lock.code("DMRGO_ERR_A"), lock.code("DMRGO_ERR_Z"); a != -100 || z != -101 {
		t.Errorf("renumbered codes %d %d, want -100 -101", a, z)
	}
	if a, b := lock.tag("u_t", "U_A"), lock.tag("u_t", "U_B"); a != 1 || b != 2 {
		t.Errorf("renumbered tags %d %d, want 1 2", a, b)
	}
	out, _, err := lock.render(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "retired") {
		t.Errorf("prerelease lock keeps retired codes: %q", out)
	}
}
