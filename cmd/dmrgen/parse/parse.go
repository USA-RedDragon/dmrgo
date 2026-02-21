// Package parse extracts dmr struct tags from Go source files and produces
// a structured model of PDU fields for code generation.
package parse

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
	FieldInt                        // bits:S-E,signed → int
	FieldEnum                       // bits:S-E,enum → custom enum type with FromInt
	FieldRaw                        // bits:S-E,raw → [N]bit.Bit sub-array
	FieldDelegate                   // bits:S-E,delegate → sub-struct with DecodeFromBits
	FieldPacked                     // bits:S-E,packed → packed bytes
	FieldLongitude                  // bits:S-E,type:longitude → float32
	FieldLatitude                   // bits:S-E,type:latitude → float32
	FieldDispatch                   // bits:S-E,dispatch:Field=Val1|Val2 → pointer to sub-PDU
)

// Field is a single tagged field within a PDU struct.
type Field struct {
	Name     string    // Go field name
	Kind     FieldKind // how to decode/encode
	BitStart int       // first bit index (inclusive)
	BitEnd   int       // last bit index (inclusive)
	BitWidth int       // BitEnd - BitStart + 1 (for single bit, width=1)

	// Type information
	GoType      string // e.g. "int", "uint8", "enums.LCSS", "elements.DataType"
	TypePkg     string // package path for qualified types (e.g. "enums")
	TypeName    string // unqualified type name (e.g. "LCSS")
	IsQualified bool   // true if GoType contains a package qualifier

	// Pointer type information (for dispatch fields)
	IsPointer   bool   // true if GoType starts with "*"
	PointedType string // type without "*" prefix (e.g. "BSOutboundActivationPDU")

	// Enum-specific
	EnumFromInt    string // e.g. "enums.LCSSFromInt", "enums.FLCOFromInt"
	EnumReturnsErr bool   // true if FromInt returns (T, error), false if just T

	// Delegate-specific
	DelegateNoPtr bool // true if the constructor returns a value (not a pointer)

	// Dispatch-specific
	DispatchFieldName string   // field name to switch on (e.g. "CSBKOpcode")
	DispatchValues    []string // constant names (e.g. ["CSBKBSOutboundActivationPDU"])

	// For non-contiguous fields: additional bit ranges
	// e.g. "bits:3+12-15" → ExtraBitRanges = [{12,15}]
	ExtraBitRanges [][2]int
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

// PDUStruct represents a tagged DMR PDU struct ready for code generation.
type PDUStruct struct {
	Name       string        // struct name
	Package    string        // Go package name
	InputSize  int           // total bit width (e.g. 20, 16, 96)
	Fields     []Field       // tagged fields in declaration order
	FEC        *FECDirective // optional FEC pre-processing
	CRC        *CRCDirective // optional CRC validation
	SpecRef    string        // ETSI spec section reference from comment
	SourceFile string        // source file path
}

// ParseFile parses a Go source file and returns all PDU structs with dmr tags.
func ParseFile(filePath string) ([]PDUStruct, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}

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

			// Check if any field has a dmr tag
			var fields []Field
			hasDMRTags := false
			for _, field := range structType.Fields.List {
				if field.Tag == nil {
					continue
				}
				tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
				dmrTag := tag.Get("dmr")
				if dmrTag == "" || dmrTag == "-" {
					continue
				}
				hasDMRTags = true

				if len(field.Names) == 0 {
					continue // embedded field
				}

				f, err := parseTag(field.Names[0].Name, dmrTag, field.Type)
				if err != nil {
					return nil, fmt.Errorf("field %s.%s: %w", typeSpec.Name.Name, field.Names[0].Name, err)
				}
				fields = append(fields, f)
			}

			if !hasDMRTags {
				continue
			}

			pdu := PDUStruct{
				Name:       typeSpec.Name.Name,
				Package:    node.Name.Name,
				Fields:     fields,
				SourceFile: filepath.Base(filePath),
			}

			// Calculate input size from field ranges
			maxBit := 0
			for _, f := range fields {
				if f.BitEnd > maxBit {
					maxBit = f.BitEnd
				}
				for _, r := range f.ExtraBitRanges {
					if r[1] > maxBit {
						maxBit = r[1]
					}
				}
			}
			pdu.InputSize = maxBit + 1

			// Look for FEC directive, CRC directive, input_size, and spec ref in preceding comments
			if genDecl.Doc != nil {
				for _, c := range genDecl.Doc.List {
					text := strings.TrimPrefix(c.Text, "//")
					text = strings.TrimSpace(text)
					if strings.HasPrefix(text, "dmr:fec ") {
						codec := strings.TrimPrefix(text, "dmr:fec ")
						pdu.FEC = &FECDirective{Codec: strings.TrimSpace(codec)}
					}
					if strings.HasPrefix(text, "dmr:crc ") {
						algo := strings.TrimPrefix(text, "dmr:crc ")
						if pdu.CRC == nil {
							pdu.CRC = &CRCDirective{}
						}
						pdu.CRC.Algorithm = strings.TrimSpace(algo)
					}
					if strings.HasPrefix(text, "dmr:crc_mask ") {
						maskStr := strings.TrimPrefix(text, "dmr:crc_mask ")
						maskStr = strings.TrimSpace(maskStr)
						mask, err := strconv.ParseUint(strings.TrimPrefix(maskStr, "0x"), 16, 16)
						if err == nil {
							if pdu.CRC == nil {
								pdu.CRC = &CRCDirective{}
							}
							pdu.CRC.Mask = uint16(mask)
							pdu.CRC.HasMask = true
						}
					}
					if strings.HasPrefix(text, "dmr:input_size ") {
						sizeStr := strings.TrimPrefix(text, "dmr:input_size ")
						size, err := strconv.Atoi(strings.TrimSpace(sizeStr))
						if err == nil {
							pdu.InputSize = size
						}
					}
					if strings.Contains(text, "ETSI TS") {
						pdu.SpecRef = text
					}
				}
			}

			result = append(result, pdu)
		}
	}

	return result, nil
}

// parseTag parses a single dmr struct tag into a Field.
func parseTag(fieldName, tag string, fieldType ast.Expr) (Field, error) {
	f := Field{Name: fieldName}

	// Resolve Go type
	f.GoType = typeString(fieldType)

	// Detect pointer types (for dispatch fields)
	if strings.HasPrefix(f.GoType, "*") {
		f.IsPointer = true
		f.PointedType = strings.TrimPrefix(f.GoType, "*")
	}

	// Resolve package qualifier from the base type (without pointer)
	baseType := f.GoType
	if f.IsPointer {
		baseType = f.PointedType
	}
	if idx := strings.LastIndex(baseType, "."); idx >= 0 {
		f.TypePkg = baseType[:idx]
		f.TypeName = baseType[idx+1:]
		f.IsQualified = true
	} else {
		f.TypeName = baseType
	}

	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return f, fmt.Errorf("empty dmr tag")
	}

	// Parse the bit specifier (first part)
	bitSpec := parts[0]
	switch {
	case strings.HasPrefix(bitSpec, "bit:"):
		// Single bit → bool
		idx, err := strconv.Atoi(strings.TrimPrefix(bitSpec, "bit:"))
		if err != nil {
			return f, fmt.Errorf("invalid bit index: %w", err)
		}
		f.Kind = FieldBool
		f.BitStart = idx
		f.BitEnd = idx
		f.BitWidth = 1

	case strings.HasPrefix(bitSpec, "bits:"):
		rangeStr := strings.TrimPrefix(bitSpec, "bits:")
		// Check for non-contiguous: "3+12-15"
		if strings.Contains(rangeStr, "+") {
			segments := strings.Split(rangeStr, "+")
			// First segment
			start, end, err := parseBitRange(segments[0])
			if err != nil {
				return f, fmt.Errorf("invalid bit range: %w", err)
			}
			f.BitStart = start
			f.BitEnd = end
			f.BitWidth = end - start + 1
			// Additional segments
			for _, seg := range segments[1:] {
				s, e, err := parseBitRange(seg)
				if err != nil {
					return f, fmt.Errorf("invalid extra bit range: %w", err)
				}
				f.ExtraBitRanges = append(f.ExtraBitRanges, [2]int{s, e})
				f.BitWidth += e - s + 1
			}
		} else {
			start, end, err := parseBitRange(rangeStr)
			if err != nil {
				return f, fmt.Errorf("invalid bit range: %w", err)
			}
			f.BitStart = start
			f.BitEnd = end
			f.BitWidth = end - start + 1
		}

		// Default kind based on Go type
		f.Kind = classifyFieldKind(f.GoType, f.BitWidth)

	default:
		return f, fmt.Errorf("unrecognized bit specifier: %q", bitSpec)
	}

	// Parse modifiers (remaining parts)
	for _, mod := range parts[1:] {
		switch {
		case mod == "enum":
			f.Kind = FieldEnum
		case mod == "raw":
			f.Kind = FieldRaw
		case mod == "delegate":
			f.Kind = FieldDelegate
		case mod == "packed":
			f.Kind = FieldPacked
		case mod == "signed":
			f.Kind = FieldInt
		case mod == "noptr":
			f.DelegateNoPtr = true
		case mod == "err":
			f.EnumReturnsErr = true
		case strings.HasPrefix(mod, "type:"):
			semType := strings.TrimPrefix(mod, "type:")
			switch semType {
			case "longitude":
				f.Kind = FieldLongitude
			case "latitude":
				f.Kind = FieldLatitude
			default:
				return f, fmt.Errorf("unknown semantic type: %q", semType)
			}
		case strings.HasPrefix(mod, "from:"):
			// Explicit FromInt function: from:enums.LCSSFromInt
			f.EnumFromInt = strings.TrimPrefix(mod, "from:")
		case strings.HasPrefix(mod, "dispatch:"):
			// Dispatch modifier: dispatch:FieldName=Value1|Value2
			f.Kind = FieldDispatch
			dispatchSpec := strings.TrimPrefix(mod, "dispatch:")
			eqIdx := strings.Index(dispatchSpec, "=")
			if eqIdx < 0 {
				return f, fmt.Errorf("dispatch modifier missing '=': %q", mod)
			}
			f.DispatchFieldName = dispatchSpec[:eqIdx]
			f.DispatchValues = strings.Split(dispatchSpec[eqIdx+1:], "|")
		default:
			return f, fmt.Errorf("unrecognized modifier: %q", mod)
		}
	}

	// For enum fields, derive the FromInt function if not explicit
	if f.Kind == FieldEnum && f.EnumFromInt == "" {
		f.EnumFromInt = f.TypePkg + "." + f.TypeName + "FromInt"
	}

	return f, nil
}

// parseBitRange parses "S-E" into (start, end) or a single number "N" into (N, N).
func parseBitRange(s string) (int, int, error) {
	if idx := strings.Index(s, "-"); idx >= 0 {
		start, err := strconv.Atoi(s[:idx])
		if err != nil {
			return 0, 0, err
		}
		end, err := strconv.Atoi(s[idx+1:])
		if err != nil {
			return 0, 0, err
		}
		return start, end, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, err
	}
	return n, n, nil
}

// classifyFieldKind guesses the appropriate FieldKind based on Go type name.
func classifyFieldKind(goType string, _ int) FieldKind {
	switch goType {
	case "bool":
		return FieldBool
	case "int":
		return FieldInt
	case "uint8", "uint16", "uint32", "byte":
		return FieldUint
	default:
		// Could be an enum or other qualified type — default to uint
		return FieldUint
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
