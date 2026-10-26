package fuzztest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Corpus reads each file of testdata/fuzz/name, which must be a "go
// test fuzz v1" input of len(dst) values. It stores the values in dst,
// which holds pointers to uint8, uint32, bool or []byte, and then
// calls each. A file that does not match fails t.
func Corpus(t testing.TB, name string, each func(), dst ...any) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata/fuzz", name, "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != len(dst)+1 || lines[0] != "go test fuzz v1" {
			t.Fatalf("%s: not a fuzz input of %d values", f, len(dst))
		}
		for i, line := range lines[1:] {
			v, err := value(line)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			if !assign(dst[i], v) {
				t.Fatalf("%s: value %d is %T, want %T", f, i, v, dst[i])
			}
		}
		each()
	}
}

func value(line string) (any, error) {
	open := strings.IndexByte(line, '(')
	if open < 0 || !strings.HasSuffix(line, ")") {
		return nil, fmt.Errorf("line %q is not a typed value", line)
	}
	typ, arg := line[:open], line[open+1:len(line)-1]
	switch typ {
	case "[]byte":
		s, err := strconv.Unquote(arg)
		return []byte(s), err
	case "bool":
		return strconv.ParseBool(arg)
	case "uint32":
		v, err := strconv.ParseUint(arg, 0, 32)
		return uint32(v), err
	case "uint8":
		v, err := strconv.ParseUint(arg, 0, 8)
		return uint8(v), err
	case "byte":
		if len(arg) < 2 || arg[0] != '\'' || arg[len(arg)-1] != '\'' {
			v, err := strconv.ParseUint(arg, 0, 8)
			return uint8(v), err
		}
		c, _, tail, err := strconv.UnquoteChar(arg[1:len(arg)-1], '\'')
		if err != nil {
			return nil, err
		}
		if tail != "" || c < 0 || c > 0xFF {
			return nil, fmt.Errorf("byte literal %s", arg)
		}
		return uint8(c), nil
	}
	return nil, fmt.Errorf("type %s", typ)
}

func assign(dst, v any) bool {
	ok := false
	switch d := dst.(type) {
	case *uint8:
		*d, ok = v.(uint8)
	case *uint32:
		*d, ok = v.(uint32)
	case *bool:
		*d, ok = v.(bool)
	case *[]byte:
		*d, ok = v.([]byte)
	}
	return ok
}
