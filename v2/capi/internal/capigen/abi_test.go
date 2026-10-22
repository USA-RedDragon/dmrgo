package capigen

import "testing"

func TestLockScalarTypedef(t *testing.T) {
	t.Parallel()

	lock, err := parseLock([]byte("abi 1\ntype struct x_t { uint8_t a; uint16_t b; };\n"))
	if err != nil {
		t.Fatal(err)
	}
	syms := []string{"sym typedef uint8_t e_t;", "sym typedef uint16_t w_t;", "type struct x_t { e_t a; uint16_t b; };"}
	out, broken, err := lock.render(syms, false)
	if err != nil || len(broken) != 0 {
		t.Fatalf("render = %v, %v", broken, err)
	}
	if string(out[:6]) != "abi 1\n" {
		t.Errorf("lock %q", out)
	}
	lock, _ = parseLock([]byte("abi 1\ntype struct x_t { uint8_t a; uint16_t b; };\n"))
	if _, broken, err := lock.render([]string{"sym typedef uint16_t w_t;", "type struct x_t { w_t a; uint16_t b; };"}, false); err == nil || len(broken) != 1 {
		t.Fatalf("a wider typedef: %v, %v", broken, err)
	}
}

func TestLockScalarTypedefPrototype(t *testing.T) {
	t.Parallel()

	lock, err := parseLock([]byte("abi 1\nsym int f(x_t *self, int64_t now);\n"))
	if err != nil {
		t.Fatal(err)
	}
	syms := []string{"sym typedef int64_t dmrgo_duration_t;", "sym int f(x_t *self, dmrgo_duration_t now);"}
	if _, broken, err := lock.render(syms, false); err != nil || len(broken) != 0 {
		t.Fatalf("render = %v, %v", broken, err)
	}
	lock, _ = parseLock([]byte("abi 1\nsym int f(x_t *self, int64_t now);\n"))
	if _, broken, err := lock.render([]string{"sym typedef int32_t d_t;", "sym int f(x_t *self, d_t now);"}, false); err == nil || len(broken) != 1 {
		t.Fatalf("a narrower typedef: %v, %v", broken, err)
	}
}

func TestLockPrefix(t *testing.T) {
	t.Parallel()

	lock, err := parseLock([]byte("abi 1\nprefix m/a/pdu dmrgo_pdu_\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p, err := lock.prefix("m/a/pdu", func() string { return "dmrgo_a_pdu_" }); err != nil || p != "dmrgo_pdu_" {
		t.Errorf("locked prefix = %q, %v", p, err)
	}
	if p, err := lock.prefix("m/b/pdu", func() string { return "dmrgo_b_pdu_" }); err != nil || p != "dmrgo_b_pdu_" {
		t.Errorf("new prefix = %q, %v", p, err)
	}
	if _, err := lock.prefix("m/c/pdu", func() string { return "dmrgo_pdu_" }); err == nil {
		t.Error("a new package took a locked prefix")
	}
	out, _, err := lock.render(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "abi 1\nprefix m/a/pdu dmrgo_pdu_\nprefix m/b/pdu dmrgo_b_pdu_\n"; string(out) != want {
		t.Errorf("lock %q, want %q", out, want)
	}
}
