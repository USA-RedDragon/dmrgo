package nilguard

import (
	"strings"
	"testing"
)

func TestApply(t *testing.T) {
	t.Parallel()

	dir := "p"
	src := `package p

type T struct{ n int }

type K uint8

func (t *T) String() string { return "t" }

func (t *T) Append(dst []byte) ([]byte, error) { return dst, nil }

func (t *T) Get() (K, bool, [2]byte) { return 0, true, [2]byte{} }

func (*T) Reset() {}

func (t *T) Done() bool {
	if t == nil {
		return false
	}
	return true
}

func (t *T) unexported() {}
`
	out, n, err := New(dir).Apply([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("%d guards, want 4:\n%s", n, out)
	}
	for _, want := range []string{
		"if t == nil {\n\t\treturn \"<nil>\"\n\t}",
		"if t == nil {\n\t\treturn dst, dmrgo.ErrNilReceiver\n\t}",
		"if t == nil {\n\t\treturn 0, false, [2]byte{}\n\t}",
		"func (r *T) Reset() {\n\tif r == nil {\n\t\treturn\n\t}",
		"import \"github.com/USA-RedDragon/dmrgo/v2\"",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
