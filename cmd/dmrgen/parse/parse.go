// Package parse extracts dmr struct tags from Go source files and produces
// a structured model of PDU fields for code generation.
package parse

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// FieldKind classifies how a PDU field should be decoded/encoded.
type FieldKind int

// The FieldKind values, with the dmr tag form of each.
const (
	FieldBool      FieldKind = iota // bit:N to bool
	FieldUint                       // bits:S-E to uint8/uint16/uint32
	FieldInt                        // bits:S-E to int (unsigned unless Signed)
	FieldEnum                       // bits:S-E,enum to named integer type with a String method
	FieldRaw                        // bits:S-E,raw to [N]byte, MSB-first, zero padding bits at the end
	FieldDelegate                   // bits:S-E,delegate to sub-struct with DecodeFromBits
	FieldPacked                     // bits:S-E,packed to packed bytes
	FieldLongitude                  // bits:S-E,type:longitude to float64
	FieldLatitude                   // bits:S-E,type:latitude to float64
)

const (
	litTrue     = "true"
	litFalse    = "false"
	typeBool    = "bool"
	typeUint    = "uint"
	typeInt     = "int"
	typeFloat64 = "float64"
	typeInt64   = "int64"
	typeStr     = "string"
)

// CRCWidths maps each supported CRC algorithm to its width in bits.
var CRCWidths = map[string]int{
	"crc_ccitt": 16,
	"crc_8":     8,
}

var specRefRe = regexp.MustCompile(`\((ETSI TS [^()]*)\)`)

// Guard is a comparison of a struct field against a literal value.
type Guard struct {
	Field string // field name (e.g. "TrunkingMode")
	Op    string // "==" or "!="
	Value string // "true", "false", or a decimal integer
}

// Field is a single tagged field within a PDU struct.
type Field struct {
	Name     string    // Go field name
	Kind     FieldKind // how to decode/encode
	BitStart int       // first bit index (inclusive)
	BitEnd   int       // last bit index (inclusive)
	BitWidth int       // total width across all ranges

	GoType      string // e.g. "int", "uint8", "enums.LCSS", "enums.DataType"
	TypePkg     string // package path for qualified types (e.g. "enums")
	TypeName    string // unqualified type name (e.g. "LCSS")
	IsQualified bool   // true if GoType contains a package qualifier
	ArrayLen    int    // element count when GoType is a fixed array, else 0

	IsPointer   bool   // true if GoType starts with "*"
	PointedType string // type without "*" prefix (e.g. "BSOutboundActivationPDU")

	// Signed marks a two's complement integer field.
	Signed bool

	// StringMethod is the method of the struct that String calls for the
	// text of the field (string:method), in place of the field's own
	// String method.
	StringMethod string

	DelegateNoPtr bool // true if the delegate constructor returns a value
	Stride        int  // for array delegates: each element spans Stride bits

	// OnlyField and OnlyValues come from only:<Field>=<Value>[|<Value>] on
	// a dmr:get getter: the getter returns the bits only when OnlyField
	// holds one of the constants, and zero otherwise.
	OnlyField  string
	OnlyValues []string

	// Union marks the raw field of a tagged union: it holds the payload
	// bits, and the dmr:variant directives give the views of them.
	Union bool

	// ExtraBitRanges holds the bit ranges after the first of a non-contiguous
	// field: "bits:3+12-15" gives [{12,15}].
	ExtraBitRanges [][2]int

	// Skip marks a dmr:"-" field, which decode and encode skip. String prints
	// it only when it is the FEC field or a dmr:input.
	Skip bool

	// NoEncode marks a field that decode reads (for example for a when:
	// guard) and encode skips, because the sub-PDU encode writes its bits.
	NoEncode bool

	// Tail marks a packed byte field at the end of the longest frame that a
	// shorter frame cuts: decode and encode copy the octets that the frame
	// holds.
	Tail bool

	// BaseType is the resolved underlying Go basic type (e.g. "uint8"), or "" if unknown.
	BaseType string

	// Encoding is how an integer field is stored in a dmr:frame or
	// dmr:view struct: big-endian bits, little-endian octets, or ASCII
	// decimal or hexadecimal digits.
	Encoding Encoding

	// Bias is added on decode and subtracted on encode (bias:N).
	Bias uint64

	// Min and Max are the values that a dmr:frame integer field takes
	// (min:N, max:N): the decoder and the encoder return a range error for
	// another value. HasRange is true when either is set.
	Min, Max uint64
	HasRange bool

	// Swap16 marks a packed field whose octet pairs are swapped (swap16).
	Swap16 bool

	// Invert marks a bool field that is true when its bit is 0 (invert).
	Invert bool

	// NulZero marks a dec field that reads as 0 when all its octets are
	// NUL (nulzero).
	NulZero bool

	// Text marks a packed field of a dmr:view that holds ASCII text padded
	// with spaces (text).
	Text bool

	// Doc is the doc comment of the field, for the methods of a dmr:view,
	// or the description of a dmr:get getter.
	Doc string

	// Getter marks a dmr:get getter: a method that reads the bits from the
	// union field.
	Getter bool
}

// Encoding is how an integer field is stored.
type Encoding int

// The Encoding values, with the dmr tag modifier of each.
const (
	EncodingBits    Encoding = iota // big-endian bits (no modifier)
	EncodingLE                      // le: little-endian octets
	EncodingDecimal                 // dec: ASCII decimal digits, one per octet
	EncodingHex                     // hex: ASCII hexadecimal digits, one per octet
)

// Ranges returns all bit ranges of the field, MSB-first.
func (f *Field) Ranges() [][2]int {
	return append([][2]int{{f.BitStart, f.BitEnd}}, f.ExtraBitRanges...)
}

// FECDirective describes a struct-level FEC pre-processing step.
type FECDirective struct {
	Codec string // e.g. "golay_20_8_7", "quadratic_residue_16_7_6", "reed_solomon_12_9_4"
}

// CRCDirective describes a struct-level CRC validation step.
type CRCDirective struct {
	Algorithm string // "crc_ccitt"
	Mask      uint16 // XOR mask applied to last 2 bytes before check (0 = no mask)
	HasMask   bool   // true if a mask was specified
}

// FECMask is one entry of a dmr:fec_mask directive.
type FECMask struct {
	Value string // constant compared against the selector field
	Mask  uint64 // XOR mask applied to the FEC parity
}

// FECMaskDirective selects an FEC parity XOR mask by the value of a field.
type FECMaskDirective struct {
	Field string    // name of the selector field
	Masks []FECMask // mask per selector value
	Err   string    // from err:Name, the exported error that Encode returns for a value without a mask
}

// ConstDirective is a dmr:const directive: bits that Encode sets to a fixed
// value and Decode ignores, such as the outer header's Format bits of a
// data header sub-PDU.
type ConstDirective struct {
	Ranges [][2]int // bit ranges, MSB-first
	Width  int      // total width across all ranges
	Value  string   // integer literal or constant name
}

// FrameDirective is a dmr:frame directive.
type FrameDirective struct {
	Sizes     []int  // allowed frame lengths in octets, ascending
	SizeField string // the dmr:"-" field that holds the length, when there is more than one
	Open      bool   // a + after the last size: a longer frame decodes too, its octets after the last size are ignored, and Size 0 means the last size
	Default   int    // the length that a zero SizeField means: the size marked with *, else the first
}

// ViewDirective is a dmr:view directive.
type ViewDirective struct {
	Type string // the byte array type that gets the methods
	Size int    // its length in octets
}

// MagicDirective is a dmr:magic directive: fixed octets at a byte offset.
type MagicDirective struct {
	Start int    // first bit, a multiple of 8
	Bytes []byte // the octets
}

// VariantCond is one condition of a dmr:variant directive: Field=V1|V2 holds when
// Field equals one of the constants Values, and Field==N or Field!=N is the
// comparison Guard.
type VariantCond struct {
	Field  string
	Values []string
	Guard  *Guard
}

// Variant is a dmr:variant directive: the payload type Type is the view of
// the union field when all Conds hold. The generator writes AsName and
// SetName methods for it.
type Variant struct {
	Name  string
	Type  string
	Conds []VariantCond
}

// PDUStruct represents a tagged DMR PDU struct ready for code generation.
type PDUStruct struct {
	Name         string            // struct name
	Package      string            // Go package name
	InputSize    int               // total bit width (e.g. 20, 16, 96)
	Fields       []Field           // tagged fields in declaration order
	FEC          *FECDirective     // optional FEC pre-processing
	CRC          *CRCDirective     // optional CRC validation
	FECMask      *FECMaskDirective // optional per-value FEC parity mask
	Consts       []ConstDirective  // bits with a fixed value
	Inputs       []string          // dmr:"-" fields passed to Decode as parameters
	DispatchIf   *Guard            // condition of every variant that has no condition on its field
	SpecRef      string            // first parenthesized ETSI citation of the doc comment
	SourceFile   string            // source file path
	HasInputSize bool              // true if dmr:input_size was given
	SubPDU       bool              // true if dmr:subpdu was given
	Payload      bool              // true if dmr:payload was given: the frame is the payload of a message

	// FieldName is set by dmr:field: the struct is a field of fewer than 8
	// bits of another PDU, named FieldName in the docs, and gets a Field
	// method and an XOf function that convert it to and from the field
	// value in the low bits of a uint8.
	FieldName string

	// Frame is set by dmr:frame: the struct is a byte frame of one of
	// Frame.Sizes octets, decoded from a slice with an error and encoded with
	// AppendBinary.
	Frame *FrameDirective

	// View is set by dmr:view: the struct only describes the layout of the
	// named byte array type, which gets a getter and setter per field.
	View *ViewDirective

	// Stream is set by dmr:stream or dmr:tokens: the struct is a sequence
	// of octet fields whose lengths depend on the data.
	Stream *StreamDirective

	// Magic holds the dmr:magic directives: octets that decode checks and
	// encode writes.
	Magic []MagicDirective

	// Variants holds the dmr:variant directives of a tagged union.
	Variants []Variant

	// Resets holds the dmr:reset Field=Value directives: every SetX
	// method sets Field to Value.
	Resets []VariantCond

	// Builds holds the dmr:build Field=Value directives: the value
	// constructor of each variant type starts from the zero struct with
	// Field set to Value.
	Builds []VariantCond

	// StringAs holds the dmr:string_as directives: hand-written AsX
	// methods that String tries, in order, after the variants.
	StringAs []string

	// Getters holds the dmr:get directives: methods that read bits of the
	// union field, such as a selector that lies inside the payload. Their
	// bit ranges count from the start of the PDU.
	Getters []Field

	// UnexportedDecode holds the names of the structs of the package whose
	// Decode function is unexported: the variant types and the dmr:subpdu
	// structs. SetUnexportedDecode fills it.
	UnexportedDecode map[string]bool

	// UnexportedEncode holds the names of the variant types and of the
	// dmr:subpdu structs without a CRC or FEC of the package, whose Encode
	// is the unexported encode method that the parent calls.
	UnexportedEncode map[string]bool

	// Types holds the structs of the package by name, for the variant
	// types. SetUnexportedDecode fills it.
	Types map[string]*PDUStruct
}

// EncodeFuncName returns the name of the encode method of the struct
// typeName of the package of p: encode for a variant type or sub-PDU,
// Encode otherwise.
func (p *PDUStruct) EncodeFuncName(typeName string) string {
	if p.UnexportedEncode[typeName] {
		return "encode"
	}
	return "Encode"
}

// UnionField returns the union field of p, or nil.
func (p *PDUStruct) UnionField() *Field {
	for i := range p.Fields {
		if p.Fields[i].Union {
			return &p.Fields[i]
		}
	}
	return nil
}

// GetterByName returns the dmr:get getter with the given name, or nil.
func (p *PDUStruct) GetterByName(name string) *Field {
	for i := range p.Getters {
		if p.Getters[i].Name == name {
			return &p.Getters[i]
		}
	}
	return nil
}

// DecodeFuncName returns the name of the Decode function of the struct
// typeName of the package of p: decodeX for a sub-PDU, DecodeX otherwise.
func (p *PDUStruct) DecodeFuncName(typeName string) string {
	if !token.IsExported(typeName) {
		return "decode" + strings.ToUpper(typeName[:1]) + typeName[1:]
	}
	if p.UnexportedDecode[typeName] {
		return "decode" + typeName
	}
	return "Decode" + typeName
}

// SetUnexportedDecode marks the Decode function of every sub-PDU of pdus,
// the structs of one package, as unexported: the variant types and the
// structs with a dmr:subpdu directive. Users reach a sub-PDU through the
// As method of its parent. It returns an error when a variant type is not
// a struct of the package or its size differs from the union field.
func SetUnexportedDecode(pdus []PDUStruct) error {
	sub := map[string]bool{}
	enc := map[string]bool{}
	byName := map[string]*PDUStruct{}
	for i := range pdus {
		byName[pdus[i].Name] = &pdus[i]
	}
	for _, p := range pdus {
		if p.SubPDU {
			sub[p.Name] = true
			enc[p.Name] = p.CRC == nil && p.FEC == nil
		}
		for _, v := range p.Variants {
			t, ok := byName[v.Type]
			if !ok {
				return fmt.Errorf("struct %s variant %s: type %s is not a PDU struct of the package", p.Name, v.Name, v.Type)
			}
			u := p.UnionField()
			if t.InputSize != u.BitWidth {
				return fmt.Errorf("struct %s variant %s: type %s has %d bits, union field %s has %d", p.Name, v.Name, v.Type, t.InputSize, u.Name, u.BitWidth)
			}
			if t.FEC != nil || t.CRC != nil || len(t.Inputs) > 0 || t.Stream != nil || t.Frame != nil || t.View != nil {
				return fmt.Errorf("struct %s variant %s: type %s must be a plain PDU struct", p.Name, v.Name, v.Type)
			}
			sub[v.Type] = true
			enc[v.Type] = true
		}
		if p.Stream != nil {
			for _, f := range p.Stream.Fields {
				if f.SKind == StreamFrame || f.SKind == StreamNested || f.SKind == StreamTokens {
					sub[f.TypeName] = true
				}
			}
		}
	}
	for i := range pdus {
		pdus[i].UnexportedDecode = sub
		pdus[i].UnexportedEncode = enc
		pdus[i].Types = byName
	}
	return nil
}

// FieldByName returns the field with the given name, or nil.
func (p *PDUStruct) FieldByName(name string) *Field {
	for i := range p.Fields {
		if p.Fields[i].Name == name {
			return &p.Fields[i]
		}
	}
	return nil
}

// IsInput reports whether name is declared with dmr:input.
func (p *PDUStruct) IsInput(name string) bool {
	for _, in := range p.Inputs {
		if in == name {
			return true
		}
	}
	return false
}

// File parses a Go source file and returns all PDU structs with dmr tags.
func File(filePath string) ([]PDUStruct, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}
	return parseAST(node, filePath, packageBaseTypes(filePath, node), newConstValues(node, filePath, true))
}

// Source parses Go source text; only types declared in src are resolved.
func Source(fileName, src string) ([]PDUStruct, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fileName, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", fileName, err)
	}
	types := map[string]string{}
	collectBaseTypes(node, types)
	return parseAST(node, fileName, types, newConstValues(node, fileName, false))
}

func parseAST(node *ast.File, filePath string, baseTypes map[string]string, consts *constValues) ([]PDUStruct, error) {
	var result []PDUStruct
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			pdu, found, err := parseStruct(typeSpec.Name.Name, structType, genDecl.Doc, baseTypes)
			if err != nil {
				return nil, fmt.Errorf("struct %s: %w", typeSpec.Name.Name, err)
			}
			if !found {
				continue
			}
			if err := checkOverlaps(&pdu, consts); err != nil {
				return nil, fmt.Errorf("struct %s: %w", typeSpec.Name.Name, err)
			}
			pdu.Package = node.Name.Name
			pdu.SourceFile = filepath.Base(filePath)
			result = append(result, pdu)
		}
	}
	return result, nil
}

func parseStruct(name string, structType *ast.StructType, doc *ast.CommentGroup, baseTypes map[string]string) (PDUStruct, bool, error) {
	if hasStreamDirective(doc) {
		pdu, err := parseStreamStruct(name, structType, doc, baseTypes)
		return pdu, err == nil, err
	}
	pdu := PDUStruct{Name: name}
	hasDMRTags := false
	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
		dmrTag, ok := tag.Lookup("dmr")
		if !ok {
			continue
		}
		hasDMRTags = true
		if len(field.Names) == 0 {
			return pdu, false, fmt.Errorf("embedded field %s has a dmr tag", typeString(field.Type))
		}
		if len(field.Names) > 1 {
			return pdu, false, fmt.Errorf("dmr tag on multi-name field %s", field.Names[0].Name)
		}
		fieldName := field.Names[0].Name
		fieldDoc := ""
		if field.Doc != nil {
			fieldDoc = strings.TrimSpace(field.Doc.Text())
		}
		if dmrTag == "-" {
			f := Field{Name: fieldName, Skip: true}
			resolveType(&f, typeString(field.Type), baseTypes)
			pdu.Fields = append(pdu.Fields, f)
			continue
		}
		f, err := parseTag(fieldName, dmrTag, typeString(field.Type), baseTypes)
		if err != nil {
			return pdu, false, fmt.Errorf("field %s: %w", fieldName, err)
		}
		if f.OnlyField != "" {
			return pdu, false, fmt.Errorf("field %s: only: is valid only on dmr:get", fieldName)
		}
		f.Doc = fieldDoc
		pdu.Fields = append(pdu.Fields, f)
	}
	if !hasDMRTags {
		return pdu, false, nil
	}

	maxBit := -1
	for _, f := range pdu.Fields {
		if f.Skip {
			continue
		}
		for _, r := range f.Ranges() {
			if r[1] > maxBit {
				maxBit = r[1]
			}
		}
	}
	pdu.InputSize = maxBit + 1

	if doc != nil {
		var prose []string
		for _, c := range doc.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if text == "dmr:subpdu" {
				pdu.SubPDU = true
				continue
			}
			if text == "dmr:payload" {
				pdu.Payload = true
				continue
			}
			if strings.HasPrefix(text, "dmr:") {
				if err := parseDirective(&pdu, text, baseTypes); err != nil {
					return pdu, false, err
				}
				continue
			}
			prose = append(prose, text)
		}
		if m := specRefRe.FindStringSubmatch(strings.Join(prose, " ")); m != nil {
			pdu.SpecRef = m[1]
		}
	}

	if err := validateLayout(&pdu, maxBit); err != nil {
		return pdu, false, err
	}
	if err := validateStruct(&pdu, maxBit); err != nil {
		return pdu, false, err
	}
	return pdu, true, nil
}

func parseDirective(pdu *PDUStruct, text string, baseTypes map[string]string) error {
	name, arg, _ := strings.Cut(text, " ")
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return fmt.Errorf("directive %q missing argument", name)
	}
	switch name {
	case "dmr:fec":
		if pdu.FEC != nil {
			return errors.New("duplicate dmr:fec directive")
		}
		pdu.FEC = &FECDirective{Codec: arg}
	case "dmr:crc", "dmr:crc_mask":
		return parseCRCDirective(pdu, name, arg)
	case "dmr:input_size":
		size, err := strconv.Atoi(arg)
		if err != nil || size <= 0 {
			return fmt.Errorf("invalid dmr:input_size %q", arg)
		}
		if size < pdu.InputSize {
			return fmt.Errorf("dmr:input_size %d is smaller than the field range (%d bits)", size, pdu.InputSize)
		}
		pdu.InputSize = size
		pdu.HasInputSize = true
	case "dmr:input":
		for _, in := range strings.Fields(arg) {
			f := pdu.FieldByName(in)
			if f == nil || !f.Skip {
				return fmt.Errorf("dmr:input %q must name a dmr:\"-\" field", in)
			}
			if pdu.IsInput(in) {
				return fmt.Errorf("duplicate dmr:input %q", in)
			}
			pdu.Inputs = append(pdu.Inputs, in)
		}
	case "dmr:field":
		pdu.FieldName = arg
	case "dmr:const":
		c, err := parseConst(arg)
		if err != nil {
			return fmt.Errorf("dmr:const: %w", err)
		}
		pdu.Consts = append(pdu.Consts, c)
	case directiveFrame, directiveView, directiveMagic:
		return parseLayoutDirective(pdu, name, arg)
	case "dmr:dispatch_if", "dmr:variant", "dmr:get", "dmr:reset", "dmr:build", "dmr:string_as":
		return parseUnionDirective(pdu, name, arg, baseTypes)
	case "dmr:fec_mask":
		if pdu.FECMask != nil {
			return errors.New("duplicate dmr:fec_mask directive")
		}
		dir, err := parseFECMask(arg)
		if err != nil {
			return err
		}
		pdu.FECMask = dir
	default:
		return fmt.Errorf("unknown directive %q", name)
	}
	return nil
}

func parseUnionDirective(pdu *PDUStruct, name, arg string, baseTypes map[string]string) error {
	switch name {
	case "dmr:dispatch_if":
		if pdu.DispatchIf != nil {
			return errors.New("duplicate dmr:dispatch_if directive")
		}
		g, err := parseGuard(arg)
		if err != nil {
			return fmt.Errorf("dmr:dispatch_if: %w", err)
		}
		pdu.DispatchIf = g
	case "dmr:reset":
		field, value, ok := strings.Cut(arg, "=")
		if !ok || !token.IsIdentifier(field) || !isQualifiedIdent(value) {
			return fmt.Errorf("dmr:reset %q is not Field=Value", arg)
		}
		pdu.Resets = append(pdu.Resets, VariantCond{Field: field, Values: []string{value}})
	case "dmr:build":
		field, value, ok := strings.Cut(arg, "=")
		if !ok || !token.IsIdentifier(field) || !isQualifiedIdent(value) {
			return fmt.Errorf("dmr:build %q is not Field=Value", arg)
		}
		pdu.Builds = append(pdu.Builds, VariantCond{Field: field, Values: []string{value}})
	case "dmr:string_as":
		if !token.IsExported(arg) {
			return fmt.Errorf("dmr:string_as %q is not an exported name", arg)
		}
		pdu.StringAs = append(pdu.StringAs, arg)
	case "dmr:variant":
		v, err := parseVariant(arg)
		if err != nil {
			return fmt.Errorf("dmr:variant: %w", err)
		}
		pdu.Variants = append(pdu.Variants, v)
	default:
		g, err := parseGetter(arg, baseTypes)
		if err != nil {
			return fmt.Errorf("dmr:get: %w", err)
		}
		pdu.Getters = append(pdu.Getters, g)
	}
	return nil
}

func parseVariant(arg string) (Variant, error) {
	parts := strings.Fields(arg)
	if len(parts) < 3 {
		return Variant{}, fmt.Errorf("expected Name Type VariantCond..., got %q", arg)
	}
	v := Variant{Name: parts[0], Type: parts[1]}
	if !token.IsExported(v.Name) || !token.IsIdentifier(v.Type) {
		return v, fmt.Errorf("bad name %q or type %q", v.Name, v.Type)
	}
	for _, c := range parts[2:] {
		cond, err := parseVariantCond(c)
		if err != nil {
			return v, err
		}
		if slices.ContainsFunc(v.Conds, func(d VariantCond) bool { return d.Field == cond.Field }) {
			return v, fmt.Errorf("variant %s has two conditions on %s", v.Name, cond.Field)
		}
		v.Conds = append(v.Conds, cond)
	}
	return v, nil
}

func parseVariantCond(s string) (VariantCond, error) {
	if strings.Contains(s, "==") || strings.Contains(s, "!=") {
		g, err := parseGuard(s)
		if err != nil {
			return VariantCond{}, err
		}
		return VariantCond{Field: g.Field, Guard: g}, nil
	}
	field, values, ok := strings.Cut(s, "=")
	if !ok || field == "" || values == "" {
		return VariantCond{}, fmt.Errorf("condition must be Field=Value[|Value], Field==N or Field!=N: %q", s)
	}
	c := VariantCond{Field: field, Values: strings.Split(values, "|")}
	if slices.Contains(c.Values, "") {
		return c, fmt.Errorf("empty value in %q", s)
	}
	return c, nil
}

func parseGetter(arg string, baseTypes map[string]string) (Field, error) {
	parts := strings.Fields(arg)
	if len(parts) < 4 {
		return Field{}, fmt.Errorf("expected Name Type Tag Description, got %q", arg)
	}
	f, err := parseTag(parts[0], parts[2], parts[1], baseTypes)
	if err != nil {
		return f, fmt.Errorf("getter %s: %w", parts[0], err)
	}
	switch f.Kind {
	case FieldBool, FieldUint, FieldEnum:
	case FieldInt, FieldRaw, FieldDelegate, FieldPacked, FieldLongitude, FieldLatitude:
		return f, fmt.Errorf("getter %s must be a bool or an unsigned integer", f.Name)
	}
	if f.Signed || f.Encoding != EncodingBits || f.Bias != 0 || f.StringMethod != "" {
		return f, fmt.Errorf("getter %s takes no signed, encoding, bias or string modifier", f.Name)
	}
	f.Doc = strings.Join(parts[3:], " ")
	f.Getter = true
	return f, nil
}

func parseCRCDirective(pdu *PDUStruct, name, arg string) error {
	if pdu.CRC == nil {
		pdu.CRC = &CRCDirective{}
	}
	if name == "dmr:crc_mask" {
		mask, err := parseHex(arg, 16)
		if err != nil || mask > math.MaxUint16 {
			return fmt.Errorf("invalid dmr:crc_mask %q: %w", arg, err)
		}
		pdu.CRC.Mask = uint16(mask)
		pdu.CRC.HasMask = true
		return nil
	}
	if _, ok := CRCWidths[arg]; !ok {
		return fmt.Errorf("unknown CRC algorithm %q", arg)
	}
	if pdu.CRC.Algorithm != "" {
		return errors.New("duplicate dmr:crc directive")
	}
	pdu.CRC.Algorithm = arg
	return nil
}

func parseConst(arg string) (ConstDirective, error) {
	var c ConstDirective
	spec, val, ok := strings.Cut(arg, "=")
	if !ok || spec == "" || val == "" {
		return c, fmt.Errorf("expected S-E[+S-E]=Value, got %q", arg)
	}
	for _, seg := range strings.Split(spec, "+") {
		s, e, err := parseBitRange(seg)
		if err != nil {
			return c, fmt.Errorf("invalid bit range %q: %w", seg, err)
		}
		c.Ranges = append(c.Ranges, [2]int{s, e})
		c.Width += e - s + 1
	}
	if c.Width > 32 {
		return c, fmt.Errorf("width %d is wider than 32 bits", c.Width)
	}
	c.Value = val
	if n, err := strconv.ParseUint(val, 0, 64); err == nil {
		if n>>c.Width != 0 {
			return c, fmt.Errorf("value %s does not fit in %d bits", val, c.Width)
		}
		return c, nil
	}
	if !isQualifiedIdent(val) {
		return c, fmt.Errorf("value %q is not an integer or a constant name", val)
	}
	return c, nil
}

func isQualifiedIdent(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return false
	}
	for _, p := range parts {
		if !token.IsIdentifier(p) {
			return false
		}
	}
	return true
}

func parseFECMask(arg string) (*FECMaskDirective, error) {
	parts := strings.Fields(arg)
	if len(parts) < 2 {
		return nil, fmt.Errorf("dmr:fec_mask needs a field and at least one Value=Mask pair: %q", arg)
	}
	dir := &FECMaskDirective{Field: parts[0]}
	for _, p := range parts[1:] {
		if name, ok := strings.CutPrefix(p, "err:"); ok {
			if !token.IsExported(name) || dir.Err != "" {
				return nil, fmt.Errorf("dmr:fec_mask entry %q is not one exported error name", p)
			}
			dir.Err = name
			continue
		}
		val, maskStr, ok := strings.Cut(p, "=")
		if !ok || val == "" {
			return nil, fmt.Errorf("dmr:fec_mask entry %q is not Value=Mask", p)
		}
		mask, err := parseHex(maskStr, 64)
		if err != nil {
			return nil, fmt.Errorf("dmr:fec_mask entry %q: %w", p, err)
		}
		dir.Masks = append(dir.Masks, FECMask{Value: val, Mask: mask})
	}
	return dir, nil
}

func parseHex(s string, bitSize int) (uint64, error) {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return 0, errors.New("must start with 0x")
	}
	return strconv.ParseUint(s[2:], 16, bitSize)
}

func parseGuard(spec string) (*Guard, error) {
	var g Guard
	switch {
	case strings.Contains(spec, "=="):
		g.Field, g.Value, _ = strings.Cut(spec, "==")
		g.Op = "=="
	case strings.Contains(spec, "!="):
		g.Field, g.Value, _ = strings.Cut(spec, "!=")
		g.Op = "!="
	default:
		return nil, fmt.Errorf("missing operator (== or !=): %q", spec)
	}
	if g.Field == "" {
		return nil, fmt.Errorf("missing field name: %q", spec)
	}
	if g.Value != litTrue && g.Value != litFalse {
		if _, err := strconv.Atoi(g.Value); err != nil {
			return nil, fmt.Errorf("value %q must be true, false, or a decimal integer", g.Value)
		}
	}
	return &g, nil
}

func parseTag(fieldName, tag, goType string, baseTypes map[string]string) (Field, error) {
	f := Field{Name: fieldName}
	resolveType(&f, goType, baseTypes)

	parts := strings.Split(tag, ",")
	if err := parseBitSpec(&f, parts[0]); err != nil {
		return f, err
	}

	var seen modifierSet
	for _, mod := range parts[1:] {
		if err := applyModifier(&f, mod, &seen); err != nil {
			return f, err
		}
	}

	if seen.semType && f.Signed {
		return f, errors.New("type:latitude/longitude are already signed; drop the signed modifier")
	}
	return f, validateField(&f)
}

type modifierSet struct {
	semType bool
}

func applyModifier(f *Field, mod string, seen *modifierSet) error {
	name, arg, hasArg := strings.Cut(mod, ":")
	if !hasArg {
		return applyFlag(f, mod)
	}
	switch name {
	case "type":
		seen.semType = true
		switch arg {
		case "longitude":
			f.Kind = FieldLongitude
		case "latitude":
			f.Kind = FieldLatitude
		default:
			return fmt.Errorf("unknown semantic type: %q", arg)
		}
	case "stride":
		stride, err := strconv.Atoi(arg)
		if err != nil || stride <= 0 {
			return fmt.Errorf("invalid stride: %q", mod)
		}
		f.Stride = stride
	case "only":
		field, values, ok := strings.Cut(arg, "=")
		if !ok || field == "" || values == "" {
			return fmt.Errorf("only modifier must be only:Field=Value[|Value]: %q", mod)
		}
		f.OnlyField = field
		f.OnlyValues = strings.Split(values, "|")
		if slices.Contains(f.OnlyValues, "") {
			return fmt.Errorf("empty only value in %q", mod)
		}
	case "min", "max":
		v, err := strconv.ParseUint(arg, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid %s: %q", name, mod)
		}
		if !f.HasRange {
			f.Min, f.Max, f.HasRange = 0, math.MaxUint64, true
		}
		if name == "min" {
			f.Min = v
		} else {
			f.Max = v
		}
	case "string":
		if !token.IsIdentifier(arg) {
			return fmt.Errorf("string modifier must name a method: %q", mod)
		}
		if f.StringMethod != "" {
			return fmt.Errorf("duplicate string modifier: %q", mod)
		}
		f.StringMethod = arg
	case "bias":
		bias, err := strconv.ParseUint(arg, 10, 32)
		if err != nil || bias == 0 {
			return fmt.Errorf("invalid bias: %q", mod)
		}
		f.Bias = bias
	default:
		return fmt.Errorf("unrecognized modifier: %q", mod)
	}
	return nil
}

func applyFlag(f *Field, mod string) error {
	switch mod {
	case "enum":
		f.Kind = FieldEnum
	case "raw":
		f.Kind = FieldRaw
	case "delegate":
		f.Kind = FieldDelegate
	case "packed":
		f.Kind = FieldPacked
	case "signed":
		f.Signed = true
	case "noptr":
		f.DelegateNoPtr = true
	case "no_encode":
		f.NoEncode = true
	case "union":
		f.Union = true
	case "le":
		f.Encoding = EncodingLE
	case "dec":
		f.Encoding = EncodingDecimal
	case "hex":
		f.Encoding = EncodingHex
	case "swap16":
		f.Swap16 = true
	case "invert":
		f.Invert = true
	case "nulzero":
		f.NulZero = true
	case "text":
		f.Text = true
	case "tail":
		f.Tail = true
	default:
		return fmt.Errorf("unrecognized modifier: %q", mod)
	}
	return nil
}

func parseBitSpec(f *Field, bitSpec string) error {
	switch {
	case strings.HasPrefix(bitSpec, "bit:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(bitSpec, "bit:"))
		if err != nil || idx < 0 {
			return fmt.Errorf("invalid bit index: %q", bitSpec)
		}
		f.Kind = FieldBool
		f.BitStart = idx
		f.BitEnd = idx
		f.BitWidth = 1
		if f.GoType != typeBool {
			return fmt.Errorf("bit:N requires a bool field, got %s", f.GoType)
		}
	case strings.HasPrefix(bitSpec, "bits:"):
		segments := strings.Split(strings.TrimPrefix(bitSpec, "bits:"), "+")
		for i, seg := range segments {
			s, e, err := parseBitRange(seg)
			if err != nil {
				return fmt.Errorf("invalid bit range %q: %w", seg, err)
			}
			if i == 0 {
				f.BitStart, f.BitEnd = s, e
			} else {
				f.ExtraBitRanges = append(f.ExtraBitRanges, [2]int{s, e})
			}
			f.BitWidth += e - s + 1
		}
		f.Kind = classifyFieldKind(f.BaseType)
	default:
		return fmt.Errorf("unrecognized bit specifier: %q", bitSpec)
	}
	return nil
}

func validateField(f *Field) error {
	if err := validateModifiers(f); err != nil {
		return err
	}
	if err := validateEncoding(f); err != nil {
		return err
	}
	nonContiguous := len(f.ExtraBitRanges) > 0
	if nonContiguous && !allowsNonContiguous(f) {
		return fmt.Errorf("%s fields must be contiguous", kindName(f))
	}
	isArray := f.ArrayLen > 0

	switch f.Kind {
	case FieldUint, FieldInt, FieldEnum:
		if isArray || f.IsPointer {
			return fmt.Errorf("integer field cannot have type %s", f.GoType)
		}
		return checkIntWidth(f)
	case FieldBool:
		if f.BitWidth != 1 {
			return errors.New("bool fields must be a single bit")
		}
	case FieldRaw:
		n := RawBytes(f.BitWidth)
		if f.GoType != fmt.Sprintf("[%d]byte", n) && f.GoType != fmt.Sprintf("[%d]uint8", n) {
			return fmt.Errorf("raw field of %d bits must be [%d]byte, got %s", f.BitWidth, n, f.GoType)
		}
	case FieldPacked:
		return validatePacked(f)
	case FieldDelegate:
		return validateDelegate(f)
	case FieldLongitude, FieldLatitude:
		if f.GoType != typeFloat64 {
			return fmt.Errorf("latitude/longitude fields must be float64, got %s", f.GoType)
		}
		if f.BitWidth < 2 || f.BitWidth > 32 {
			return fmt.Errorf("latitude/longitude width %d out of range 2-32", f.BitWidth)
		}
	}
	return nil
}

func validatePacked(f *Field) error {
	isArray := f.ArrayLen > 0
	if !isArray && f.IsQualified && !f.IsPointer && f.BitWidth%8 == 0 {
		return nil
	}
	if !isArray || (f.GoType != fmt.Sprintf("[%d]byte", f.ArrayLen) && f.GoType != fmt.Sprintf("[%d]uint8", f.ArrayLen)) {
		return fmt.Errorf("packed field must be [N]byte, got %s", f.GoType)
	}
	if f.ArrayLen*8 != f.BitWidth && (f.Encoding != EncodingHex || f.ArrayLen*16 != f.BitWidth) {
		return fmt.Errorf("packed array of %d bytes does not match range width %d", f.ArrayLen, f.BitWidth)
	}
	return nil
}

// RawBytes returns the number of bytes of a raw field of width bits: the
// bits fill the bytes most significant bit first, and the padding bits of
// the last byte are zero.
func RawBytes(width int) int {
	return (width + 7) / 8
}

func validateModifiers(f *Field) error {
	switch {
	case f.DelegateNoPtr && f.Kind != FieldDelegate:
		return errors.New("noptr requires delegate")
	case f.Stride > 0 && f.Kind != FieldDelegate:
		return errors.New("stride requires delegate")
	case f.Union && f.Kind != FieldRaw:
		return errors.New("union requires raw")
	case f.Invert && f.Kind != FieldBool:
		return errors.New("invert requires a bool field")
	case f.Union && f.NoEncode:
		return errors.New("union and no_encode cannot be combined")
	case f.StringMethod != "" && (f.Union || f.Stride > 0):
		return errors.New("string: cannot be combined with union or stride")
	case f.OnlyField != "" && f.Kind != FieldUint && f.Kind != FieldInt && f.Kind != FieldEnum:
		return errors.New("only: requires an integer field")
	case f.Signed && (f.Kind != FieldInt || !isSignedBase(f.BaseType)):
		return fmt.Errorf("signed requires a signed integer field, got %s", f.GoType)
	case f.Signed && f.BitWidth < 2:
		return errors.New("signed fields need at least 2 bits")
	}
	return nil
}

func allowsNonContiguous(f *Field) bool {
	switch f.Kind {
	case FieldUint, FieldInt, FieldEnum:
		return true
	case FieldDelegate:
		return f.DelegateNoPtr && f.Stride == 0
	case FieldBool, FieldRaw, FieldPacked, FieldLongitude, FieldLatitude:
	}
	return false
}

func kindName(f *Field) string {
	switch f.Kind {
	case FieldBool:
		return typeBool
	case FieldRaw:
		return "raw"
	case FieldPacked:
		return "packed"
	case FieldDelegate:
		if f.Stride > 0 {
			return "stride delegate"
		}
		return "struct delegate"
	case FieldLongitude, FieldLatitude:
		return "latitude/longitude"
	case FieldUint, FieldInt, FieldEnum:
	}
	return "integer"
}

func validateDelegate(f *Field) error {
	isArray := f.ArrayLen > 0
	switch {
	case f.Stride > 0:
		if f.BitWidth%f.Stride != 0 {
			return fmt.Errorf("stride %d does not divide range width %d", f.Stride, f.BitWidth)
		}
		if !isArray || f.ArrayLen != f.BitWidth/f.Stride {
			return fmt.Errorf("stride delegate type %s does not hold %d elements", f.GoType, f.BitWidth/f.Stride)
		}
	case f.DelegateNoPtr:
		if isArray || f.IsPointer {
			return fmt.Errorf("noptr delegate cannot have type %s", f.GoType)
		}
		return checkIntWidth(f)
	default:
		if isArray || f.IsPointer {
			return fmt.Errorf("struct delegate cannot have type %s", f.GoType)
		}
	}
	return nil
}

func checkIntWidth(f *Field) error {
	if f.Encoding == EncodingDecimal {
		return nil
	}
	if f.Encoding == EncodingHex {
		g := *f
		g.BitWidth = f.DigitWidth()
		g.Encoding = EncodingBits
		return checkIntWidth(&g)
	}
	if f.BitWidth > 64 {
		return fmt.Errorf("integer fields wider than 64 bits are not supported (%d)", f.BitWidth)
	}
	switch f.BaseType {
	case "", typeInt, "uint", typeInt64, "uint64", "uintptr":
		return nil
	case "float32", typeFloat64, typeBool, typeStr, "complex64", "complex128":
		return fmt.Errorf("type %s cannot hold an integer field", f.GoType)
	}
	if w := basicWidth(f.BaseType); w > 0 && f.BitWidth > w {
		return fmt.Errorf("type %s (%d bits) is narrower than the %d-bit range", f.GoType, w, f.BitWidth)
	}
	return nil
}

func validateStruct(pdu *PDUStruct, maxBit int) error {
	if maxBit >= pdu.InputSize {
		return fmt.Errorf("fields extend to bit %d beyond input size %d", maxBit, pdu.InputSize)
	}
	for _, c := range pdu.Consts {
		for _, r := range c.Ranges {
			if r[1] >= pdu.InputSize {
				return fmt.Errorf("dmr:const bits %d-%d extend beyond input size %d", r[0], r[1], pdu.InputSize)
			}
		}
	}

	if pdu.FieldName != "" && (pdu.InputSize >= 8 || pdu.FEC != nil || pdu.CRC != nil || len(pdu.Inputs) > 0 || len(pdu.Variants) > 0) {
		return errors.New("dmr:field needs a struct of fewer than 8 bits without FEC, CRC, inputs or variants")
	}
	if err := checkUnion(pdu); err != nil {
		return err
	}
	if err := checkOverlap(pdu); err != nil {
		return err
	}
	if err := checkCRCRange(pdu); err != nil {
		return err
	}
	if err := checkCoverage(pdu); err != nil {
		return err
	}
	if err := checkFECField(pdu); err != nil {
		return err
	}
	if pdu.FECMask != nil {
		if !pdu.IsInput(pdu.FECMask.Field) {
			return fmt.Errorf("dmr:fec_mask field %q must be a dmr:input, since FEC runs before field decode", pdu.FECMask.Field)
		}
		if pdu.FEC == nil {
			return errors.New("dmr:fec_mask requires dmr:fec")
		}
	}
	return nil
}

func checkUnion(pdu *PDUStruct) error {
	var union *Field
	for i := range pdu.Fields {
		if !pdu.Fields[i].Union {
			continue
		}
		if union != nil {
			return fmt.Errorf("multiple union fields: %s and %s", union.Name, pdu.Fields[i].Name)
		}
		union = &pdu.Fields[i]
	}
	switch {
	case union == nil && (len(pdu.Variants) > 0 || len(pdu.Getters) > 0 || pdu.DispatchIf != nil || len(pdu.Resets) > 0 || len(pdu.Builds) > 0 || len(pdu.StringAs) > 0):
		return errors.New("dmr:variant, dmr:get, dmr:reset, dmr:build, dmr:string_as and dmr:dispatch_if need a union field")
	case union == nil:
		return nil
	case len(pdu.Variants) == 0:
		return fmt.Errorf("union field %s without dmr:variant directives", union.Name)
	}
	if err := checkResets(pdu); err != nil {
		return err
	}
	for i, g := range pdu.Getters {
		if pdu.FieldByName(g.Name) != nil || slices.ContainsFunc(pdu.Getters[:i], func(h Field) bool { return h.Name == g.Name }) {
			return fmt.Errorf("getter %s has the name of a field or getter", g.Name)
		}
		for _, r := range g.Ranges() {
			if r[0] < union.BitStart || r[1] > union.BitEnd {
				return fmt.Errorf("getter %s bits %d-%d are outside union field %s", g.Name, r[0], r[1], union.Name)
			}
		}
		if g.OnlyField != "" {
			c := VariantCond{Field: g.OnlyField, Values: g.OnlyValues}
			if err := checkCond(pdu, c, pdu.Getters[:i]); err != nil {
				return fmt.Errorf("getter %s only: %w", g.Name, err)
			}
		}
	}
	for i := range pdu.Variants {
		v := &pdu.Variants[i]
		if slices.ContainsFunc(pdu.Variants[:i], func(w Variant) bool { return w.Name == v.Name }) {
			return fmt.Errorf("duplicate variant %s", v.Name)
		}
		if pdu.DispatchIf != nil && !slices.ContainsFunc(v.Conds, func(c VariantCond) bool { return c.Field == pdu.DispatchIf.Field }) {
			v.Conds = append(v.Conds, VariantCond{Field: pdu.DispatchIf.Field, Guard: pdu.DispatchIf})
		}
		for _, c := range v.Conds {
			if err := checkCond(pdu, c, pdu.Getters); err != nil {
				return fmt.Errorf("variant %s: %w", v.Name, err)
			}
		}
	}
	return nil
}

func checkResets(pdu *PDUStruct) error {
	for _, r := range pdu.Resets {
		f := pdu.FieldByName(r.Field)
		if f == nil || f.Union || f.GoType == typeBool {
			return fmt.Errorf("dmr:reset field %q is not an integer field of the struct", r.Field)
		}
	}
	for i, b := range pdu.Builds {
		f := pdu.FieldByName(b.Field)
		if f == nil || f.Union || slices.ContainsFunc(pdu.Builds[:i], func(c VariantCond) bool { return c.Field == b.Field }) {
			return fmt.Errorf("dmr:build field %q is not a field of the struct, or is the union or set twice", b.Field)
		}
	}
	for _, name := range pdu.StringAs {
		if slices.ContainsFunc(pdu.Variants, func(v Variant) bool { return v.Name == name }) {
			return fmt.Errorf("dmr:string_as %s is also a variant", name)
		}
	}
	return nil
}

func checkCond(pdu *PDUStruct, c VariantCond, getters []Field) error {
	f := pdu.FieldByName(c.Field)
	if f == nil {
		for i := range getters {
			if getters[i].Name == c.Field {
				f = &getters[i]
			}
		}
	}
	switch {
	case f == nil:
		return fmt.Errorf("unknown field %q", c.Field)
	case f.Skip && !pdu.IsInput(c.Field):
		return fmt.Errorf("field %q is dmr:\"-\" and not a dmr:input, so it is never set during decode", c.Field)
	case f.Union:
		return fmt.Errorf("field %q is the union field", c.Field)
	case c.Guard != nil && (f.GoType == typeBool) != (c.Guard.Value == litTrue || c.Guard.Value == litFalse):
		return fmt.Errorf("value %s does not match type %s of %s", c.Guard.Value, f.GoType, c.Field)
	case c.Guard == nil && f.GoType == typeBool:
		return fmt.Errorf("bool field %s needs %s==true or %s==false", c.Field, c.Field, c.Field)
	}
	return nil
}

func checkOverlap(pdu *PDUStruct) error {
	owner := map[int]string{}
	for _, f := range pdu.Fields {
		if f.Skip || f.NoEncode {
			continue
		}
		for _, r := range f.Ranges() {
			for b := r[0]; b <= r[1]; b++ {
				if prev, ok := owner[b]; ok {
					return fmt.Errorf("fields %s and %s overlap at bit %d", prev, f.Name, b)
				}
				owner[b] = f.Name
			}
		}
	}
	for _, c := range pdu.Consts {
		for _, r := range c.Ranges {
			for b := r[0]; b <= r[1]; b++ {
				if prev, ok := owner[b]; ok {
					return fmt.Errorf("%s and dmr:const overlap at bit %d", prev, b)
				}
				owner[b] = "dmr:const"
			}
		}
	}
	for _, m := range pdu.Magic {
		for b := m.Start; b < m.Start+8*len(m.Bytes); b++ {
			if prev, ok := owner[b]; ok {
				return fmt.Errorf("%s and dmr:magic overlap at bit %d", prev, b)
			}
			owner[b] = "dmr:magic"
		}
	}
	return nil
}

func checkCoverage(pdu *PDUStruct) error {
	covered := make([]bool, pdu.InputSize)
	mark := func(r [2]int) {
		for b := r[0]; b <= r[1]; b++ {
			covered[b] = true
		}
	}
	for _, f := range pdu.Fields {
		if f.Skip {
			continue
		}
		for _, r := range f.Ranges() {
			mark(r)
		}
	}
	for _, c := range pdu.Consts {
		for _, r := range c.Ranges {
			mark(r)
		}
	}
	if pdu.CRC != nil {
		mark([2]int{pdu.InputSize - CRCWidths[pdu.CRC.Algorithm], pdu.InputSize - 1})
	}
	for _, m := range pdu.Magic {
		mark([2]int{m.Start, m.Start + 8*len(m.Bytes) - 1})
	}
	var gaps []string
	for b := 0; b < len(covered); b++ {
		if covered[b] {
			continue
		}
		e := b
		for e+1 < len(covered) && !covered[e+1] {
			e++
		}
		if e == b {
			gaps = append(gaps, strconv.Itoa(b))
		} else {
			gaps = append(gaps, fmt.Sprintf("%d-%d", b, e))
		}
		b = e
	}
	if len(gaps) > 0 {
		return fmt.Errorf("uncovered bits %s: add a field, the CRC or a dmr:const", strings.Join(gaps, ","))
	}
	return nil
}

func checkFECField(pdu *PDUStruct) error {
	f := pdu.FieldByName("FEC")
	hasField := f != nil && f.Skip && f.GoType == "fec.Result"
	switch {
	case f != nil && !hasField:
		return fmt.Errorf("field FEC must be a dmr:\"-\" fec.Result, got %s", f.GoType)
	case (pdu.FEC != nil || pdu.CRC != nil) && !hasField:
		return errors.New("dmr:fec and dmr:crc need a dmr:\"-\" FEC fec.Result field for the decode result")
	case pdu.FEC == nil && pdu.CRC == nil && hasField:
		return errors.New("field FEC needs dmr:fec or dmr:crc: a PDU without either has no result to report")
	}
	return nil
}

func checkCRCRange(pdu *PDUStruct) error {
	if pdu.CRC == nil {
		return nil
	}
	if pdu.CRC.Algorithm == "" {
		return errors.New("dmr:crc_mask without dmr:crc")
	}
	width := CRCWidths[pdu.CRC.Algorithm]
	crcStart := pdu.InputSize - width
	if crcStart < 0 {
		return fmt.Errorf("input size %d too small for %s", pdu.InputSize, pdu.CRC.Algorithm)
	}
	if width == 16 && pdu.InputSize%8 != 0 {
		return fmt.Errorf("byte-level CRC needs an input size divisible by 8, got %d", pdu.InputSize)
	}
	for _, f := range pdu.Fields {
		if f.Skip {
			continue
		}
		for _, r := range f.Ranges() {
			if r[1] >= crcStart {
				return fmt.Errorf("field %s (bits %d-%d) overlaps the CRC at bits %d-%d",
					f.Name, r[0], r[1], crcStart, pdu.InputSize-1)
			}
		}
	}
	return nil
}

func parseBitRange(s string) (int, int, error) {
	startStr, endStr, isRange := strings.Cut(s, "-")
	start, err := strconv.Atoi(startStr)
	if err != nil {
		return 0, 0, err
	}
	end := start
	if isRange {
		end, err = strconv.Atoi(endStr)
		if err != nil {
			return 0, 0, err
		}
	}
	if start < 0 {
		return 0, 0, fmt.Errorf("negative bit index %d", start)
	}
	if end < start {
		return 0, 0, fmt.Errorf("reversed range %d-%d", start, end)
	}
	return start, end, nil
}

func classifyFieldKind(baseType string) FieldKind {
	switch {
	case baseType == typeBool:
		return FieldBool
	case isSignedBase(baseType):
		return FieldInt
	default:
		return FieldUint
	}
}

func isSignedBase(t string) bool {
	switch t {
	case typeInt, "int8", "int16", "int32", typeInt64:
		return true
	}
	return false
}

func basicWidth(t string) int {
	switch t {
	case typeBool:
		return 1
	case "uint8", "byte", "int8":
		return 8
	case "uint16", "int16":
		return 16
	case "uint32", "int32", "rune":
		return 32
	case "uint64", typeInt64, typeInt, "uint":
		return 64
	}
	return 0
}

func resolveType(f *Field, goType string, baseTypes map[string]string) {
	f.GoType = goType
	if strings.HasPrefix(goType, "*") {
		f.IsPointer = true
		f.PointedType = strings.TrimPrefix(goType, "*")
	}
	baseType := goType
	if f.IsPointer {
		baseType = f.PointedType
	}
	if strings.HasPrefix(baseType, "[") {
		if i := strings.Index(baseType, "]"); i > 0 {
			if n, err := strconv.Atoi(baseType[1:i]); err == nil {
				f.ArrayLen = n
			}
			baseType = baseType[i+1:]
		}
	}
	if idx := strings.LastIndex(baseType, "."); idx >= 0 {
		f.TypePkg = baseType[:idx]
		f.TypeName = baseType[idx+1:]
		f.IsQualified = true
	} else {
		f.TypeName = baseType
	}
	if f.ArrayLen == 0 && !f.IsPointer && !f.IsQualified {
		f.BaseType = resolveBase(f.TypeName, baseTypes)
	}
}

func resolveBase(name string, baseTypes map[string]string) string {
	for range 8 {
		if basicWidth(name) > 0 || name == "float32" || name == typeFloat64 || name == typeStr {
			return name
		}
		next, ok := baseTypes[name]
		if !ok {
			return ""
		}
		name = next
	}
	return ""
}

func packageBaseTypes(filePath string, node *ast.File) map[string]string {
	types := map[string]string{}
	collectBaseTypes(node, types)
	dir := filepath.Dir(filePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return types
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		if path == filePath {
			continue
		}
		other, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil || other.Name.Name != node.Name.Name {
			continue
		}
		collectBaseTypes(other, types)
	}
	return types
}

func collectBaseTypes(node *ast.File, types map[string]string) {
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if id, ok := ts.Type.(*ast.Ident); ok {
				types[ts.Name.Name] = id.Name
			}
		}
	}
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.ArrayType:
		if t.Len != nil {
			if lit, ok := t.Len.(*ast.BasicLit); ok {
				return "[" + lit.Value + "]" + typeString(t.Elt)
			}
		}
		return "[]" + typeString(t.Elt)
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	default:
		return fmt.Sprintf("%T", expr)
	}
}
