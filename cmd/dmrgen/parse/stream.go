package parse

import (
	"errors"
	"fmt"
	"go/ast"
	"reflect"
	"strconv"
	"strings"
)

const (
	directiveStream = "dmr:stream"
	directiveTokens = "dmr:tokens"
	directiveBound  = "dmr:bound"
)

// StreamKind is how a field of a dmr:stream or dmr:tokens struct is stored.
type StreamKind int

// The StreamKind values, with the dmr tag form of each.
const (
	StreamInt       StreamKind = iota // u8 u16 u24 u32 u64, i8 i16 i32 i64: big-endian (le: little-endian) integer
	StreamFixed                       // i32,scale:N/E or u32,scale:N/E: integer times N/2^E as float64
	StreamUintvar                     // uintvar: base-128, most significant group first, bit 7 set on all but the last octet
	StreamSintvar                     // sintvar: uintvar whose first octet holds the sign in bit 6
	StreamUfloatvar                   // ufloatvar: uintvar integer part, then a uintvar fraction over 128 per octet
	StreamSfloatvar                   // sfloatvar: sintvar integer part, then a uintvar fraction
	StreamFrame                       // frame: a dmr:frame struct of one size
	StreamNested                      // stream: a dmr:stream struct
	StreamTokens                      // tokens: a dmr:tokens struct that takes the rest of the octets
	StreamArray                       // bytes:N: a [N]byte
	StreamBytes                       // lv8 lv16 lvvar rest len:Expr: a []byte that points into the input
	StreamFlag                        // flag: a token without a value, as a bool
)

// BytesLength is how the length of a StreamBytes field is known.
type BytesLength int

// The BytesLength values.
const (
	LengthRest   BytesLength = iota // rest: all octets up to the end
	LengthPrefix                    // lv8, lv16, lvvar: a length before the octets
	LengthExpr                      // len:Expr: computed from fields before it
)

// TextCoding is the character coding of a StreamBytes field.
type TextCoding int

// The TextCoding values.
const (
	TextNone    TextCoding = iota
	TextUTF16LE            // utf16le
	TextUTF16BE            // utf16be
)

// Cond is an if: condition on fields decoded before the field.
type Cond struct {
	Path   string   // Go selector relative to the struct, such as "Header.Extended"
	Op     string   // "" for a bool path, "==" or "!="
	Values []string // integer literals or constant names
}

// LenExpr is a len: expression: Path*Mul+Add.
type LenExpr struct {
	Path string
	Mul  int
	Add  int
}

// StreamField is one field of a dmr:stream or dmr:tokens struct.
type StreamField struct {
	Field
	SKind     StreamKind
	Octets    int  // octets of a StreamInt or StreamFixed, or of the length of lv8 (1) and lv16 (2)
	LE        bool // le
	SignedInt bool
	ScaleNum  uint64
	ScaleExp  int
	Prec      int // fraction octets that AppendBinary writes for a float varint
	Length    BytesLength
	VarLen    bool // lvvar
	Expr      *LenExpr
	Text      TextCoding
	Nul       bool // nul: the text ends with a NUL character
	If        *Cond
	Token     uint8  // token ID in a dmr:tokens struct
	HasToken  bool   // token: was given
	Has       string // the bool field that tells whether the token is present
	Bound     BoundMode
	Const     string // const:V: Decode checks the value and AppendBinary writes it
}

// BoundMode marks the integer that gives the length of a stream.
type BoundMode int

// The BoundMode values.
const (
	BoundNone  BoundMode = iota
	BoundAfter           // length: the octets after the field
	BoundTotal           // length:total: the octets from the start of the struct
)

// StreamDirective is a dmr:stream or dmr:tokens directive.
type StreamDirective struct {
	Tokens   bool // dmr:tokens
	Trailing bool // dmr:stream trailing: Decode ignores octets after the bound
	Fields   []StreamField
	// BoundPath is set by dmr:bound Path [total]: an integer of a frame
	// field that gives the length of the stream.
	BoundPath string
	BoundMode BoundMode
}

func hasStreamDirective(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		name, _, _ := strings.Cut(text, " ")
		if name == directiveStream || name == directiveTokens {
			return true
		}
	}
	return false
}

func parseStreamStruct(name string, structType *ast.StructType, doc *ast.CommentGroup, baseTypes map[string]string) (PDUStruct, error) {
	pdu := PDUStruct{Name: name, Stream: &StreamDirective{}}
	var prose []string
	for _, c := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if !strings.HasPrefix(text, "dmr:") {
			prose = append(prose, text)
			continue
		}
		dname, arg, _ := strings.Cut(text, " ")
		arg = strings.TrimSpace(arg)
		switch dname {
		case directiveStream:
			switch arg {
			case "":
			case "trailing":
				pdu.Stream.Trailing = true
			default:
				return pdu, fmt.Errorf("dmr:stream: unknown argument %q", arg)
			}
		case directiveTokens:
			if arg != "" {
				return pdu, fmt.Errorf("dmr:tokens takes no argument, got %q", arg)
			}
			pdu.Stream.Tokens = true
		case directiveBound:
			parts := strings.Fields(arg)
			if len(parts) == 0 || len(parts) > 2 || !strings.Contains(parts[0], ".") {
				return pdu, fmt.Errorf("dmr:bound wants Field.Sub [total], got %q", arg)
			}
			pdu.Stream.BoundPath = parts[0]
			pdu.Stream.BoundMode = BoundAfter
			if len(parts) == 2 {
				if parts[1] != "total" {
					return pdu, fmt.Errorf("dmr:bound: unknown mode %q", parts[1])
				}
				pdu.Stream.BoundMode = BoundTotal
			}
		default:
			return pdu, fmt.Errorf("directive %s is not valid with dmr:stream or dmr:tokens", dname)
		}
	}
	if m := specRefRe.FindStringSubmatch(strings.Join(prose, " ")); m != nil {
		pdu.SpecRef = m[1]
	}
	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		tag, ok := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Lookup("dmr")
		if !ok {
			continue
		}
		if len(field.Names) != 1 {
			return pdu, errors.New("a dmr tag needs a field with one name")
		}
		fname := field.Names[0].Name
		var sf StreamField
		sf.Name = fname
		resolveType(&sf.Field, typeString(field.Type), baseTypes)
		if field.Doc != nil {
			sf.Doc = strings.TrimSpace(field.Doc.Text())
		}
		if tag == "-" {
			sf.Skip = true
			pdu.Fields = append(pdu.Fields, sf.Field)
			pdu.Stream.Fields = append(pdu.Stream.Fields, sf)
			continue
		}
		if err := parseStreamTag(&sf, tag); err != nil {
			return pdu, fmt.Errorf("field %s: %w", fname, err)
		}
		sf.Kind = printKind(&sf)
		pdu.Fields = append(pdu.Fields, sf.Field)
		pdu.Stream.Fields = append(pdu.Stream.Fields, sf)
	}
	return pdu, validateStream(&pdu)
}

func parseStreamTag(sf *StreamField, tag string) error {
	parts := strings.Split(tag, ",")
	head, arg, _ := strings.Cut(parts[0], ":")
	switch head {
	case "u8", "u16", "u24", "u32", "u64", "i8", "i16", "i32", "i64":
		bits, _ := strconv.Atoi(head[1:])
		sf.SKind = StreamInt
		sf.Octets = bits / 8
		sf.SignedInt = head[0] == 'i'
	case "uintvar":
		sf.SKind = StreamUintvar
	case "sintvar":
		sf.SKind = StreamSintvar
	case "ufloatvar":
		sf.SKind = StreamUfloatvar
		sf.Prec = 1
	case "sfloatvar":
		sf.SKind = StreamSfloatvar
		sf.Prec = 1
	case "frame":
		sf.SKind = StreamFrame
	case "stream":
		sf.SKind = StreamNested
	case "tokens":
		sf.SKind = StreamTokens
	case "flag":
		sf.SKind = StreamFlag
	case "bytes":
		n, err := strconv.Atoi(arg)
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid bytes:%s", arg)
		}
		sf.SKind = StreamArray
		sf.Octets = n
	case "lv8", "lv16", "lvvar":
		sf.SKind = StreamBytes
		sf.Length = LengthPrefix
		switch head {
		case "lv8":
			sf.Octets = 1
		case "lv16":
			sf.Octets = 2
		default:
			sf.VarLen = true
		}
	case "rest":
		sf.SKind = StreamBytes
		sf.Length = LengthRest
	case "len":
		e, err := parseLenExpr(arg)
		if err != nil {
			return err
		}
		sf.SKind = StreamBytes
		sf.Length = LengthExpr
		sf.Expr = e
	default:
		return fmt.Errorf("unknown stream kind %q", parts[0])
	}
	if head != "bytes" && head != "len" && arg != "" {
		return fmt.Errorf("kind %s takes no argument", head)
	}
	for _, mod := range parts[1:] {
		if err := applyStreamModifier(sf, mod); err != nil {
			return err
		}
	}
	return checkStreamFieldType(sf)
}

func applyStreamModifier(sf *StreamField, mod string) error {
	name, arg, hasArg := strings.Cut(mod, ":")
	switch name {
	case "le":
		sf.LE = true
	case "utf16le":
		sf.Text = TextUTF16LE
	case "utf16be":
		sf.Text = TextUTF16BE
	case "nul":
		sf.Nul = true
	case "length":
		sf.Bound = BoundAfter
		if hasArg {
			if arg != "total" {
				return fmt.Errorf("unknown length mode %q", arg)
			}
			sf.Bound = BoundTotal
		}
	case "scale":
		n, e, ok := strings.Cut(arg, "/")
		num, err1 := strconv.ParseUint(n, 10, 32)
		exp, err2 := strconv.Atoi(e)
		if !ok || err1 != nil || err2 != nil || num == 0 || exp < 0 || exp > 63 {
			return fmt.Errorf("scale wants N/E, got %q", arg)
		}
		sf.ScaleNum, sf.ScaleExp = num, exp
		sf.SKind = StreamFixed
	case "prec":
		p, err := strconv.Atoi(arg)
		if err != nil || p < 1 || p > 4 {
			return fmt.Errorf("prec wants 1 to 4 octets, got %q", arg)
		}
		sf.Prec = p
	case "token":
		v, err := strconv.ParseUint(arg, 0, 8)
		if err != nil {
			return fmt.Errorf("invalid token %q", arg)
		}
		sf.Token, sf.HasToken = uint8(v), true
	case "if":
		c, err := parseCond(arg)
		if err != nil {
			return err
		}
		sf.If = c
	case "const":
		if !hasArg || arg == "" {
			return errors.New("const wants a value")
		}
		sf.Const = arg
	default:
		return fmt.Errorf("unknown modifier %q", mod)
	}
	return nil
}

func parseCond(arg string) (*Cond, error) {
	c := &Cond{}
	switch {
	case strings.Contains(arg, "=="):
		c.Path, arg, _ = strings.Cut(arg, "==")
		c.Op = "=="
	case strings.Contains(arg, "!="):
		c.Path, arg, _ = strings.Cut(arg, "!=")
		c.Op = "!="
	default:
		c.Path = arg
	}
	if !isPath(c.Path) {
		return nil, fmt.Errorf("if: %q is not a field path", c.Path)
	}
	if c.Op != "" {
		c.Values = strings.Split(arg, "|")
		for _, v := range c.Values {
			if _, err := strconv.ParseInt(v, 0, 64); err != nil && !isQualifiedIdent(v) {
				return nil, fmt.Errorf("if: %q is not an integer or a constant", v)
			}
		}
	}
	return c, nil
}

func isPath(s string) bool {
	if s == "" {
		return false
	}
	for _, p := range strings.Split(s, ".") {
		if !isIdent(p) {
			return false
		}
	}
	return true
}

func isIdent(s string) bool {
	return isQualifiedIdent(s) && !strings.Contains(s, ".")
}

func parseLenExpr(arg string) (*LenExpr, error) {
	e := &LenExpr{Mul: 1}
	rest := arg
	i := strings.IndexAny(rest, "*+-")
	if i < 0 {
		e.Path = rest
		rest = ""
	} else {
		e.Path, rest = rest[:i], rest[i:]
	}
	if !isPath(e.Path) {
		return nil, fmt.Errorf("len: %q is not a field path", e.Path)
	}
	if strings.HasPrefix(rest, "*") {
		j := strings.IndexAny(rest[1:], "+-")
		num := rest[1:]
		if j >= 0 {
			num, rest = rest[1:1+j], rest[1+j:]
		} else {
			rest = ""
		}
		m, err := strconv.Atoi(num)
		if err != nil || m <= 0 {
			return nil, fmt.Errorf("len: invalid factor in %q", arg)
		}
		e.Mul = m
	}
	if rest != "" {
		a, err := strconv.Atoi(rest)
		if err != nil {
			return nil, fmt.Errorf("len: invalid offset in %q", arg)
		}
		e.Add = a
	}
	return e, nil
}

func checkStreamFieldType(sf *StreamField) error {
	if err := checkStreamKindType(sf); err != nil {
		return err
	}
	return checkStreamModifiers(sf)
}

func checkStreamKindType(sf *StreamField) error {
	t := sf.GoType
	isInt := sf.BaseType != "" && basicWidth(sf.BaseType) > 0 && sf.BaseType != typeBool || (sf.IsQualified && !sf.IsPointer && sf.ArrayLen == 0)
	switch sf.SKind {
	case StreamInt:
		return checkStreamInt(sf, isInt)
	case StreamFixed:
		if t != typeFloat64 || sf.Octets == 0 || sf.Octets > 4 {
			return errors.New("scale needs a float64 field of u8 to u32 or i8 to i32")
		}
	case StreamUintvar, StreamSintvar:
		if !isInt || sf.LE {
			return fmt.Errorf("varint needs an integer type, got %s", t)
		}
		if sf.SKind == StreamSintvar && sf.BaseType != "" && !isSignedBase(sf.BaseType) {
			return fmt.Errorf("sintvar needs a signed type, got %s", t)
		}
	case StreamUfloatvar, StreamSfloatvar:
		if t != typeFloat64 {
			return fmt.Errorf("float varint needs float64, got %s", t)
		}
	case StreamFrame, StreamNested, StreamTokens:
		if sf.IsPointer || sf.ArrayLen > 0 || sf.IsQualified || sf.BaseType != "" {
			return fmt.Errorf("frame, stream and tokens need a struct type of the same package, got %s", t)
		}
	case StreamArray:
		if t != fmt.Sprintf("[%d]byte", sf.Octets) {
			return fmt.Errorf("bytes:%d needs [%d]byte, got %s", sf.Octets, sf.Octets, t)
		}
	case StreamBytes:
		if t != "[]byte" {
			return fmt.Errorf("byte string kinds need []byte, got %s", t)
		}
	case StreamFlag:
		if t != typeBool {
			return fmt.Errorf("flag needs bool, got %s", t)
		}
	}
	return nil
}

func checkStreamInt(sf *StreamField, isInt bool) error {
	t := sf.GoType
	if !isInt {
		return fmt.Errorf("integer kind needs an integer type, got %s", t)
	}
	if w := basicWidth(sf.BaseType); w > 0 && w < 8*sf.Octets {
		return fmt.Errorf("type %s is narrower than %d octets", t, sf.Octets)
	}
	if sf.SignedInt && sf.BaseType != "" && !isSignedBase(sf.BaseType) {
		return fmt.Errorf("signed kind needs a signed type, got %s", t)
	}
	return nil
}

func checkStreamModifiers(sf *StreamField) error {
	switch {
	case sf.LE && sf.SKind != StreamInt && sf.SKind != StreamFixed:
		return errors.New("le needs an integer kind")
	case sf.Nul && sf.Text == TextNone:
		return errors.New("nul needs utf16le or utf16be")
	case sf.Text != TextNone && sf.SKind != StreamBytes:
		return errors.New("utf16le and utf16be need a byte string kind")
	case sf.Bound != BoundNone && sf.SKind != StreamInt && sf.SKind != StreamUintvar:
		return errors.New("length needs an unsigned integer or uintvar kind")
	case sf.Bound != BoundNone && (sf.SignedInt || sf.If != nil || sf.Const != ""):
		return errors.New("length cannot be signed, conditional or const")
	case sf.Const != "" && sf.SKind != StreamInt:
		return errors.New("const needs an integer kind")
	case sf.Prec != 0 && sf.SKind != StreamUfloatvar && sf.SKind != StreamSfloatvar:
		return errors.New("prec needs ufloatvar or sfloatvar")
	}
	return nil
}

func validateStream(pdu *PDUStruct) error {
	st := pdu.Stream
	names := map[string]int{}
	for i, f := range st.Fields {
		names[f.Name] = i
	}
	bounds := 0
	for i := range st.Fields {
		f := &st.Fields[i]
		if f.Skip {
			continue
		}
		if st.Tokens {
			if err := checkTokenField(st, names, i); err != nil {
				return err
			}
			continue
		}
		if f.HasToken {
			return fmt.Errorf("field %s: token needs dmr:tokens", f.Name)
		}
		if f.SKind == StreamFlag {
			return fmt.Errorf("field %s: flag needs dmr:tokens", f.Name)
		}
		if isTail(f) {
			for _, g := range st.Fields[i+1:] {
				if !g.Skip && (f.If == nil || g.If == nil) {
					return fmt.Errorf("field %s: only conditional fields may follow rest or tokens, and only after a conditional one", f.Name)
				}
			}
		}
		if f.Bound != BoundNone {
			bounds++
		}
		for _, p := range []string{condPath(f.If), exprPath(f.Expr)} {
			if p == "" {
				continue
			}
			head, _, _ := strings.Cut(p, ".")
			j, ok := names[head]
			if !ok || j >= i {
				return fmt.Errorf("field %s: %s must name a field before it", f.Name, p)
			}
		}
	}
	if st.BoundPath != "" {
		bounds++
		head, _, _ := strings.Cut(st.BoundPath, ".")
		j, ok := names[head]
		if !ok || st.Fields[j].SKind != StreamFrame {
			return fmt.Errorf("dmr:bound %s must name a field of a frame field", st.BoundPath)
		}
	}
	if bounds > 1 {
		return errors.New("a stream has at most one length")
	}
	if st.Tokens && (st.BoundPath != "" || st.Trailing) {
		return errors.New("dmr:tokens cannot have dmr:bound or trailing")
	}
	return nil
}

func checkTokenField(st *StreamDirective, names map[string]int, i int) error {
	f := &st.Fields[i]
	if !f.HasToken {
		return fmt.Errorf("field %s of a dmr:tokens struct needs token:ID", f.Name)
	}
	if f.SKind == StreamTokens || (f.SKind == StreamBytes && f.Length == LengthRest) || f.Bound != BoundNone || f.If != nil {
		return fmt.Errorf("field %s: tokens, rest, length and if are not valid in a dmr:tokens struct", f.Name)
	}
	if f.SKind != StreamFlag {
		f.Has = "Has" + f.Name
		j, ok := names[f.Has]
		if !ok || !st.Fields[j].Skip || st.Fields[j].GoType != typeBool {
			return fmt.Errorf("token field %s needs a dmr:\"-\" bool field %s", f.Name, f.Has)
		}
	}
	for _, g := range st.Fields[:i] {
		if !g.Skip && g.Token == f.Token {
			return fmt.Errorf("fields %s and %s have the same token", g.Name, f.Name)
		}
	}
	return nil
}

func printKind(sf *StreamField) FieldKind {
	switch sf.SKind {
	case StreamInt, StreamUintvar, StreamSintvar:
		if sf.IsQualified || basicWidth(sf.GoType) == 0 && sf.GoType != typeUint && sf.GoType != typeInt {
			return FieldEnum
		}
		if sf.SignedInt || sf.SKind == StreamSintvar {
			return FieldInt
		}
		return FieldUint
	case StreamFixed, StreamUfloatvar, StreamSfloatvar:
		return FieldLatitude
	case StreamFrame, StreamNested, StreamTokens:
		return FieldDelegate
	case StreamArray, StreamBytes:
		return FieldPacked
	case StreamFlag:
	}
	return FieldBool
}

func isTail(f *StreamField) bool {
	return f.SKind == StreamTokens || (f.SKind == StreamBytes && f.Length == LengthRest)
}

func condPath(c *Cond) string {
	if c == nil {
		return ""
	}
	return c.Path
}

func exprPath(e *LenExpr) string {
	if e == nil {
		return ""
	}
	return e.Path
}
