package parse_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

const overlapFields = `type Sub struct {
	X uint8 ` + "`dmr:\"bits:0-7\"`" + `
}

const (
	OpA uint8 = 1
	OpB uint8 = 0x01
	OpC uint8 = 2
)
`

const overlapUnion = `type T struct {
	M   bool    ` + "`dmr:\"bit:0\"`" + `
	Op  uint8   ` + "`dmr:\"bits:1-7\"`" + `
	FID uint8   ` + "`dmr:\"bits:8-15\"`" + `
	R   [1]byte ` + "`dmr:\"bits:16-23,raw,union\"`" + `
}`

func TestParseRejectsOverlappingVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		dirs  string
		error string
	}{
		{"same value", "// dmr:variant A Sub Op=1\n// dmr:variant B Sub Op=1", "variants A and B can both match"},
		{"shared value in sets", "// dmr:variant A Sub Op=1|2\n// dmr:variant B Sub Op=2|3", "variants A and B can both match"},
		{"constant aliases", "// dmr:variant A Sub Op=OpA\n// dmr:variant B Sub Op=OpB", "variants A and B can both match"},
		{"guard and value", "// dmr:variant A Sub Op==2\n// dmr:variant B Sub Op=OpC", "variants A and B can both match"},
		{"conditions on other fields", "// dmr:variant A Sub Op=1\n// dmr:variant B Sub FID=1", "variants A and B can both match"},
		{"two not-equal guards", "// dmr:variant A Sub Op!=1\n// dmr:variant B Sub Op!=2", "variants A and B can both match"},
		{"not-equal and other values", "// dmr:variant A Sub Op!=1\n// dmr:variant B Sub Op=1|2", "variants A and B can both match"},
		{"third variant", "// dmr:variant A Sub Op=1\n// dmr:variant B Sub Op=2\n// dmr:variant C Sub Op=3 M==true\n// dmr:variant D Sub Op=OpC|3", "variants B and D can both match"},
		{"dispatch_if does not separate", "// dmr:dispatch_if FID==0\n// dmr:variant A Sub Op=1\n// dmr:variant B Sub Op=1 FID==0", "variants A and B can both match"},
		{"unknown constant", "// dmr:variant A Sub Op=Nope\n// dmr:variant B Sub Op=2", "variant A: value Nope of a variant condition is not an integer literal or a constant with one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseSrc(t, overlapFields+"\n\n"+tt.dirs+"\n"+overlapUnion)
			if err == nil || !strings.Contains(err.Error(), tt.error) {
				t.Fatalf("error %v, want %q", err, tt.error)
			}
		})
	}
}

func TestParseAcceptsDisjointVariants(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, dirs string }{
		{"different values", "// dmr:variant A Sub Op=OpA\n// dmr:variant B Sub Op=OpC"},
		{"value sets", "// dmr:variant A Sub Op=1|2\n// dmr:variant B Sub Op=3|4"},
		{"same opcode, other FID", "// dmr:dispatch_if FID==0\n// dmr:variant A Sub Op=1\n// dmr:variant B Sub Op=1 FID=16"},
		{"equal and not-equal guards", "// dmr:variant A Sub Op=1 FID==0\n// dmr:variant B Sub Op=1 FID!=0"},
		{"bool guards", "// dmr:variant A Sub M==true\n// dmr:variant B Sub M!=true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseSrc(t, overlapFields+"\n\n"+tt.dirs+"\n"+overlapUnion); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParseResolvesImportedConstants(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write := func(name, src string) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("go.mod", "module example.com/m\n\ngo 1.24\n")
	write("enums/enums.go", "package enums\n\ntype FID uint8\n\nconst (\n\tFIDStandard FID = 0\n\tFIDOther    FID = 0x10\n\tFIDAlias    FID = 16\n)\n")
	src := func(other string) string {
		return `package pdu

import "example.com/m/enums"

type Sub struct {
	X uint8 ` + "`dmr:\"bits:0-7\"`" + `
}

// dmr:variant A Sub Op=1 FID=enums.FIDOther
// dmr:variant B Sub Op=1 FID=` + other + `
type T struct {
	Op  uint8     ` + "`dmr:\"bits:0-7\"`" + `
	FID enums.FID ` + "`dmr:\"bits:8-15\"`" + `
	R   [1]byte   ` + "`dmr:\"bits:16-23,raw,union\"`" + `
}
`
	}
	ok := write("pdu/ok.go", src("enums.FIDStandard"))
	if _, err := parse.File(ok); err != nil {
		t.Fatalf("distinct imported constants: %v", err)
	}
	if err := os.Remove(ok); err != nil {
		t.Fatal(err)
	}
	bad := write("pdu/bad.go", src("enums.FIDAlias"))
	if _, err := parse.File(bad); err == nil || !strings.Contains(err.Error(), "variants A and B can both match") {
		t.Fatalf("aliased imported constants: %v", err)
	}
}
