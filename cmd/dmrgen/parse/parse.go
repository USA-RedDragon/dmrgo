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
	"strconv"
	"strings"
)

// FieldKind classifies how a PDU field should be decoded/encoded.
type FieldKind int

const (
	FieldBool      FieldKind = iota // bit:N → bool
	FieldUint                       // bits:S-E → uint8/uint16/uint32
	FieldInt                        // bits:S-E → int (unsigned unless Signed)
	FieldEnum                       // bits:S-E,enum → named integer type with a String method
	FieldRaw                        // bits:S-E,raw → [N]bit.Bit sub-array
	FieldDelegate                   // bits:S-E,delegate → sub-struct with DecodeFromBits
	FieldPacked                     // bits:S-E,packed → packed bytes
	FieldLongitude                  // bits:S-E,type:longitude → float64
	FieldLatitude                   // bits:S-E,type:latitude → float64
	FieldDispatch                   // bits:S-E,dispatch:Field=Val1|Val2 → pointer to sub-PDU
)

const (
	typeBool    = "bool"
	typeInt     = "int"
	typeFloat64 = "float64"
	typeInt64   = "int64"
)

// CRCWidths maps each supported CRC algorithm to its width in bits.
//
//nolint:gochecknoglobals
var CRCWidths = map[string]int{
	"crc_ccitt": 16,
	"crc_8":     8,
}

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

	// Type information
	GoType      string // e.g. "int", "uint8", "enums.LCSS", "enums.DataType"
	TypePkg     string // package path for qualified types (e.g. "enums")
	TypeName    string // unqualified type name (e.g. "LCSS")
	IsQualified bool   // true if GoType contains a package qualifier
	ArrayLen    int    // element count when GoType is a fixed array, else 0

	// Pointer type information (for dispatch fields)
	IsPointer   bool   // true if GoType starts with "*"
	PointedType string // type without "*" prefix (e.g. "BSOutboundActivationPDU")

	// Signed marks a two's complement integer field.
	Signed bool

	// Delegate-specific
	DelegateNoPtr bool // true if the constructor returns a value (not a pointer)
	Stride        int  // for array delegates: each element spans Stride bits

	// Dispatch-specific
	DispatchFieldName string   // field name to switch on (e.g. "CSBKOpcode")
	DispatchValues    []string // constant names (e.g. ["CSBKBSOutboundActivationPDU"])

	// When is an optional secondary guard (when:<Field><Op><Value>) that
	// disambiguates dispatch fields sharing the same primary dispatch value.
	When *Guard

	// Fallback marks a raw field that receives the dispatch payload when no
	// dispatch variant is decoded, and is encoded when no variant is set.
	Fallback bool

	// For non-contiguous fields: additional bit ranges
	// e.g. "bits:3+12-15" → ExtraBitRanges = [{12,15}]
	ExtraBitRanges [][2]int

	// Skip fields (dmr:"-") are included for String generation but excluded from decode/encode.
	Skip bool

	// NoEncode fields are decoded (e.g. for when: guard conditions) but skipped during encode
	// because the sub-PDU's encode already writes the overlapping bits.
	NoEncode bool

	// BaseType is the resolved underlying Go basic type (e.g. "uint8"), or "" if unknown.
	BaseType string
}

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
	Field string
	Masks []FECMask
}

// ConstDirective is a dmr:const directive: bits that Encode sets to a fixed
// value and Decode ignores, such as the outer header's Format bits of a
// data header sub-PDU.
type ConstDirective struct {
	Ranges [][2]int // bit ranges, MSB-first
	Width  int      // total width across all ranges
	Value  string   // integer literal or constant name
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
	DispatchIf   *Guard            // optional guard required before dispatching
	SpecRef      string            // ETSI spec section reference from comment
	SourceFile   string            // source file path
	HasInputSize bool              // true if dmr:input_size was given
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

// ParseFile parses a Go source file and returns all PDU structs with dmr tags.
func ParseFile(filePath string) ([]PDUStruct, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}
	return parseAST(node, filePath, packageBaseTypes(filePath, node))
}

// ParseSource parses Go source text; only types declared in src are resolved.
func ParseSource(fileName, src string) ([]PDUStruct, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fileName, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", fileName, err)
	}
	types := map[string]string{}
	collectBaseTypes(node, types)
	return parseAST(node, fileName, types)
}

func parseAST(node *ast.File, filePath string, baseTypes map[string]string) ([]PDUStruct, error) {
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
			pdu.Package = node.Name.Name
			pdu.SourceFile = filepath.Base(filePath)
			result = append(result, pdu)
		}
	}
	return result, nil
}

func parseStruct(name string, structType *ast.StructType, doc *ast.CommentGroup, baseTypes map[string]string) (PDUStruct, bool, error) {
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
		for _, c := range doc.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if strings.HasPrefix(text, "dmr:") {
				if err := parseDirective(&pdu, text); err != nil {
					return pdu, false, err
				}
				continue
			}
			if pdu.SpecRef == "" && strings.Contains(text, "ETSI TS") {
				pdu.SpecRef = text
			}
		}
	}

	if err := validateStruct(&pdu, maxBit); err != nil {
		return pdu, false, err
	}
	return pdu, true, nil
}

func parseDirective(pdu *PDUStruct, text string) error {
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
	case "dmr:crc":
		if _, ok := CRCWidths[arg]; !ok {
			return fmt.Errorf("unknown CRC algorithm %q", arg)
		}
		if pdu.CRC == nil {
			pdu.CRC = &CRCDirective{}
		}
		if pdu.CRC.Algorithm != "" {
			return errors.New("duplicate dmr:crc directive")
		}
		pdu.CRC.Algorithm = arg
	case "dmr:crc_mask":
		mask, err := parseHex(arg, 16)
		if err != nil || mask > math.MaxUint16 {
			return fmt.Errorf("invalid dmr:crc_mask %q: %w", arg, err)
		}
		if pdu.CRC == nil {
			pdu.CRC = &CRCDirective{}
		}
		pdu.CRC.Mask = uint16(mask)
		pdu.CRC.HasMask = true
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
	case "dmr:dispatch_if":
		if pdu.DispatchIf != nil {
			return errors.New("duplicate dmr:dispatch_if directive")
		}
		g, err := parseGuard(arg)
		if err != nil {
			return fmt.Errorf("dmr:dispatch_if: %w", err)
		}
		pdu.DispatchIf = g
	case "dmr:const":
		c, err := parseConst(arg)
		if err != nil {
			return fmt.Errorf("dmr:const: %w", err)
		}
		pdu.Consts = append(pdu.Consts, c)
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

func parseConst(arg string) (ConstDirective, error) {
	var c ConstDirective
	spec, val, ok := strings.Cut(arg, "=")
	if !ok || spec == "" || val == "" {
		return c, fmt.Errorf("want S-E[+S-E]=Value, got %q", arg)
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
	if g.Value != "true" && g.Value != "false" {
		if _, err := strconv.Atoi(g.Value); err != nil {
			return nil, fmt.Errorf("value %q must be true, false, or a decimal integer", g.Value)
		}
	}
	return &g, nil
}

// parseTag parses a single dmr struct tag into a Field.
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

// modifierSet records modifiers whose validity depends on the final kind.
type modifierSet struct {
	semType bool
}

func applyModifier(f *Field, mod string, seen *modifierSet) error {
	name, arg, hasArg := strings.Cut(mod, ":")
	if !hasArg {
		return applyFlag(f, mod, seen)
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
	case "dispatch":
		f.Kind = FieldDispatch
		field, values, ok := strings.Cut(arg, "=")
		if !ok || field == "" || values == "" {
			return fmt.Errorf("dispatch modifier must be dispatch:Field=Value[|Value]: %q", mod)
		}
		f.DispatchFieldName = field
		f.DispatchValues = strings.Split(values, "|")
		for _, v := range f.DispatchValues {
			if v == "" {
				return fmt.Errorf("empty dispatch value in %q", mod)
			}
		}
	case "when":
		g, err := parseGuard(arg)
		if err != nil {
			return fmt.Errorf("when modifier: %w", err)
		}
		f.When = g
	default:
		return fmt.Errorf("unrecognized modifier: %q", mod)
	}
	return nil
}

func applyFlag(f *Field, mod string, seen *modifierSet) error {
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
	case "fallback":
		f.Fallback = true
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
		if !isArray || f.GoType != fmt.Sprintf("[%d]bit.Bit", f.ArrayLen) {
			return fmt.Errorf("raw field must be [N]bit.Bit, got %s", f.GoType)
		}
		if f.ArrayLen != f.BitWidth {
			return fmt.Errorf("raw array length %d does not match range width %d", f.ArrayLen, f.BitWidth)
		}
	case FieldPacked:
		if !isArray || (f.GoType != fmt.Sprintf("[%d]byte", f.ArrayLen) && f.GoType != fmt.Sprintf("[%d]uint8", f.ArrayLen)) {
			return fmt.Errorf("packed field must be [N]byte, got %s", f.GoType)
		}
		if f.ArrayLen*8 != f.BitWidth {
			return fmt.Errorf("packed array of %d bytes does not match range width %d", f.ArrayLen, f.BitWidth)
		}
	case FieldDelegate:
		return validateDelegate(f)
	case FieldLongitude, FieldLatitude:
		if f.GoType != typeFloat64 {
			return fmt.Errorf("latitude/longitude fields must be float64, got %s", f.GoType)
		}
		if f.BitWidth < 2 || f.BitWidth > 32 {
			return fmt.Errorf("latitude/longitude width %d out of range 2-32", f.BitWidth)
		}
	case FieldDispatch:
		if !f.IsPointer {
			return fmt.Errorf("dispatch field must be a pointer, got %s", f.GoType)
		}
	}
	return nil
}

func validateModifiers(f *Field) error {
	switch {
	case f.DelegateNoPtr && f.Kind != FieldDelegate:
		return errors.New("noptr requires delegate")
	case f.Stride > 0 && f.Kind != FieldDelegate:
		return errors.New("stride requires delegate")
	case f.Fallback && f.Kind != FieldRaw:
		return errors.New("fallback requires raw")
	case f.When != nil && f.Kind != FieldDispatch:
		return errors.New("when: requires dispatch")
	case f.NoEncode && f.Kind == FieldDispatch:
		return errors.New("no_encode is not valid on dispatch fields")
	case f.Signed && (f.Kind != FieldInt || !isSignedBase(f.BaseType)):
		return fmt.Errorf("signed requires a signed integer field, got %s", f.GoType)
	case f.Signed && f.BitWidth < 2:
		return errors.New("signed fields need at least 2 bits")
	}
	return nil
}

// allowsNonContiguous reports whether a field kind supports A+S-E ranges.
func allowsNonContiguous(f *Field) bool {
	switch f.Kind {
	case FieldUint, FieldInt, FieldEnum:
		return true
	case FieldDelegate:
		return f.DelegateNoPtr && f.Stride == 0
	case FieldBool, FieldRaw, FieldPacked, FieldLongitude, FieldLatitude, FieldDispatch:
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
	case FieldDispatch:
		return "dispatch"
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
	if f.BitWidth > 64 {
		return fmt.Errorf("integer fields wider than 64 bits are not supported (%d)", f.BitWidth)
	}
	switch f.BaseType {
	case "", typeInt, "uint", typeInt64, "uint64", "uintptr":
		return nil
	case "float32", typeFloat64, typeBool, "string", "complex64", "complex128":
		return fmt.Errorf("type %s cannot hold an integer field", f.GoType)
	}
	if w := basicWidth(f.BaseType); w > 0 && f.BitWidth > w {
		return fmt.Errorf("type %s (%d bits) is narrower than the %d-bit range", f.GoType, w, f.BitWidth)
	}
	return nil
}

func validateStruct(pdu *PDUStruct, maxBit int) error {
	var dispatch []*Field
	var fallback *Field
	for i := range pdu.Fields {
		f := &pdu.Fields[i]
		switch {
		case f.Kind == FieldDispatch && !f.Skip:
			dispatch = append(dispatch, f)
		case f.Fallback:
			if fallback != nil {
				return fmt.Errorf("multiple fallback fields: %s and %s", fallback.Name, f.Name)
			}
			fallback = f
		}
	}

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

	if err := checkDispatch(pdu, dispatch, fallback); err != nil {
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

func checkDispatch(pdu *PDUStruct, dispatch []*Field, fallback *Field) error {
	if len(dispatch) == 0 {
		if fallback != nil {
			return fmt.Errorf("fallback field %s without dispatch fields", fallback.Name)
		}
		if pdu.DispatchIf != nil {
			return errors.New("dmr:dispatch_if without dispatch fields")
		}
		return nil
	}
	first := dispatch[0]
	for _, f := range dispatch[1:] {
		if f.BitStart != first.BitStart || f.BitEnd != first.BitEnd {
			return fmt.Errorf("dispatch field %s range %d-%d differs from %s range %d-%d",
				f.Name, f.BitStart, f.BitEnd, first.Name, first.BitStart, first.BitEnd)
		}
		if f.DispatchFieldName != first.DispatchFieldName {
			return fmt.Errorf("dispatch field %s switches on %s, %s switches on %s",
				f.Name, f.DispatchFieldName, first.Name, first.DispatchFieldName)
		}
	}
	if fallback != nil && (fallback.BitStart != first.BitStart || fallback.BitEnd != first.BitEnd) {
		return fmt.Errorf("fallback field %s range %d-%d differs from dispatch range %d-%d",
			fallback.Name, fallback.BitStart, fallback.BitEnd, first.BitStart, first.BitEnd)
	}
	if err := checkGuardField(pdu, first.DispatchFieldName); err != nil {
		return fmt.Errorf("dispatch: %w", err)
	}
	if pdu.DispatchIf != nil {
		if err := checkGuard(pdu, pdu.DispatchIf); err != nil {
			return fmt.Errorf("dmr:dispatch_if: %w", err)
		}
	}
	for _, f := range dispatch {
		if f.When != nil {
			if err := checkGuard(pdu, f.When); err != nil {
				return fmt.Errorf("field %s when: %w", f.Name, err)
			}
		}
	}
	return nil
}

func checkGuard(pdu *PDUStruct, g *Guard) error {
	if err := checkGuardField(pdu, g.Field); err != nil {
		return err
	}
	f := pdu.FieldByName(g.Field)
	isBoolLit := g.Value == "true" || g.Value == "false"
	if (f.GoType == typeBool) != isBoolLit {
		return fmt.Errorf("value %s does not match type %s of %s", g.Value, f.GoType, g.Field)
	}
	return nil
}

// checkGuardField verifies that name is set before the dispatch switch runs:
// either a regular decoded field or a declared dmr:input.
func checkGuardField(pdu *PDUStruct, name string) error {
	f := pdu.FieldByName(name)
	switch {
	case f == nil:
		return fmt.Errorf("unknown field %q", name)
	case f.Skip && !pdu.IsInput(name):
		return fmt.Errorf("field %q is dmr:\"-\" and not a dmr:input, so it is never set during decode", name)
	case f.Kind == FieldDispatch && !f.Skip:
		return fmt.Errorf("field %q is a dispatch field", name)
	}
	return nil
}

func checkOverlap(pdu *PDUStruct) error {
	owner := map[int]string{}
	for _, f := range pdu.Fields {
		if f.Skip || f.Kind == FieldDispatch || f.Fallback || f.NoEncode {
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
	return nil
}

// checkCoverage verifies that a field, the CRC or a dmr:const covers every
// input bit, so that decode then encode keeps every bit.
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

// parseBitRange parses "S-E" into (start, end) or a single number "N" into (N, N).
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

// classifyFieldKind picks the default FieldKind from the resolved base type.
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
		if basicWidth(name) > 0 || name == "float32" || name == typeFloat64 || name == "string" {
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

// packageBaseTypes collects "type X Y" declarations (Y an identifier) from
// every non-test Go file in the directory of filePath.
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

// typeString converts an ast.Expr to a Go type string.
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
