package emit

import (
	"fmt"
	"go/token"
	"math"
	"strconv"
	"strings"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
	. "github.com/dave/jennifer/jen"
)

const (
	errShort     = "ErrShort"
	errLength    = "ErrLength"
	errSignature = "ErrSignature"
	errText      = "ErrText"
)

func hasTextEncoding(pdu parse.PDUStruct) bool {
	for _, f := range pdu.Fields {
		if !f.Skip && (f.Encoding == parse.EncodingDecimal || f.Encoding == parse.EncodingHex) {
			return true
		}
	}
	return false
}

func sizesText(sizes []int) string {
	parts := make([]string, len(sizes))
	for i, s := range sizes {
		parts[i] = strconv.Itoa(s)
	}
	if len(parts) == 1 {
		return octetsText(sizes[0])
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " or " + parts[len(parts)-1] + " octets"
}

func octetsText(n int) string {
	if n == 1 {
		return "1 octet"
	}
	return strconv.Itoa(n) + " octets"
}

func magicText(pdu parse.PDUStruct) string {
	parts := make([]string, 0, len(pdu.Magic))
	for _, m := range pdu.Magic {
		a, b := m.Start/8, m.Start/8+len(m.Bytes)-1
		v := strconv.Quote(string(m.Bytes))
		if !isPrintable(m.Bytes) {
			v = fmt.Sprintf("0x%X", m.Bytes)
		}
		parts = append(parts, "octets "+strconv.Itoa(a)+" to "+strconv.Itoa(b)+" do not hold "+v)
	}
	return strings.Join(parts, " or ")
}

func isPrintable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return true
}

type sizeGroup struct {
	size   int
	fields []parse.Field
}

func sizeGroups(pdu parse.PDUStruct) []sizeGroup {
	var out []sizeGroup
	for _, field := range pdu.Fields {
		if field.Skip || field.NoEncode {
			continue
		}
		size := 0
		if s := minSize(pdu, field); !field.Tail && len(pdu.Frame.Sizes) > 1 && s > pdu.Frame.Sizes[0] {
			size = s
		}
		if len(out) > 0 && out[len(out)-1].size == size {
			out[len(out)-1].fields = append(out[len(out)-1].fields, field)
			continue
		}
		out = append(out, sizeGroup{size: size, fields: []parse.Field{field}})
	}
	return out
}

func minSize(pdu parse.PDUStruct, f parse.Field) int {
	end := 0
	for _, r := range f.Ranges() {
		end = max(end, r[1]+1)
	}
	for _, s := range pdu.Frame.Sizes {
		if s*8 >= end {
			return s
		}
	}
	return pdu.Frame.Sizes[len(pdu.Frame.Sizes)-1]
}

func rangeText(pdu parse.PDUStruct) string {
	var parts []string
	for _, f := range pdu.Fields {
		if f.Skip || f.NoEncode || !f.HasRange {
			continue
		}
		sep := " to "
		if f.Max == f.Min+1 {
			sep = " or "
		}
		parts = append(parts, f.Name+" is not "+strconv.FormatUint(f.Min, 10)+sep+strconv.FormatUint(f.Max, 10))
	}
	return strings.Join(parts, " or ")
}

func rangeCond(field parse.Field, v *Statement) *Statement {
	hi := Uint64().Call(v.Clone()).Op(">").Id(strconv.FormatUint(field.Max, 10))
	if field.Min == 0 {
		return hi
	}
	return Uint64().Call(v.Clone()).Op("<").Id(strconv.FormatUint(field.Min, 10)).Op("||").Add(hi)
}

func fieldRangeErr(ev *errVars, pdu parse.PDUStruct, field parse.Field) Code {
	return rangeErr(ev, pdu, field, Lit(float64(field.Min)), Lit(float64(field.Max)))
}

func emitFrameDecode(f *File, pdu parse.PDUStruct, ev *errVars) {
	sizes := pdu.Frame.Sizes
	name := pdu.DecodeFuncName(pdu.Name)
	doc := name + " decodes " + docObject(pdu)
	noun := " from data, a frame of "
	if pdu.Payload {
		noun = " from data, a payload of "
	}
	open, last := pdu.Frame.Open, sizes[len(sizes)-1]
	text := sizesText(sizes)
	if open {
		text += " or more"
	}
	doc = strings.TrimSuffix(doc, ".") + noun + text + ". It returns "
	conds := []string{errShort + " when data is shorter than " + octetsText(sizes[0])}
	gaps := last-sizes[0]+1 != len(sizes)
	switch {
	case open && gaps:
		conds = append(conds, errLength+" when it is shorter than "+octetsText(last)+" and its length is not one of these")
	case open:
	case len(sizes) == 1:
		conds = append(conds, errLength+" when it is longer")
	default:
		conds = append(conds, errLength+" when its length is not one of these")
	}
	if len(pdu.Magic) > 0 {
		conds = append(conds, errSignature+" when "+magicText(pdu))
	}
	if hasTextEncoding(pdu) {
		conds = append(conds, errText+" when a text number holds a character that is not a digit")
	}
	if r := rangeText(pdu); r != "" {
		conds = append(conds, "a *dmrgo.FieldRangeError when "+r)
	}
	if len(conds) == 1 {
		doc += conds[0] + "."
	} else {
		doc += strings.Join(conds[:len(conds)-1], ", ") + " and " + conds[len(conds)-1] + "."
	}
	if open {
		doc += " It ignores the octets after the first " + strconv.Itoa(last) + "."
	}
	doc += " It does not keep data."
	if token.IsExported(name) {
		comment(f, doc+docCite(pdu))
	}
	zero := Id(pdu.Name).Values()
	f.Func().Id(name).Params(Id("data").Index().Byte()).Params(Id(pdu.Name), Error()).BlockFunc(func(g *Group) {
		switch {
		case len(sizes) == 1:
			g.If(Len(Id("data")).Op("<").Lit(sizes[0])).Block(Return(zero, Id(errShort)))
			if !open {
				g.If(Len(Id("data")).Op("!=").Lit(sizes[0])).Block(Return(zero, Id(errLength)))
			}
		case open && !gaps:
			g.If(Len(Id("data")).Op("<").Lit(sizes[0])).Block(Return(zero, Id(errShort)))
		default:
			g.Switch(Len(Id("data"))).BlockFunc(func(sw *Group) {
				vals := make([]Code, len(sizes))
				for i, s := range sizes {
					vals[i] = Lit(s)
				}
				sw.Case(vals...)
				sw.Default().BlockFunc(func(d *Group) {
					d.If(Len(Id("data")).Op("<").Lit(sizes[0])).Block(Return(zero, Id(errShort)))
					if open {
						d.If(Len(Id("data")).Op("<").Lit(last)).Block(Return(zero, Id(errLength)))
					} else {
						d.Return(zero, Id(errLength))
					}
				})
			})
		}
		for _, m := range pdu.Magic {
			var cond *Statement
			for i, b := range m.Bytes {
				c := Id("data").Index(Lit(m.Start/8 + i)).Op("!=").Add(hexByte(b))
				if cond == nil {
					cond = c
				} else {
					cond = cond.Op("||").Add(c)
				}
			}
			g.If(cond).Block(Return(zero, Id(errSignature)))
		}
		g.Var().Id("result").Id(pdu.Name)
		if sf := pdu.Frame.SizeField; sf != "" {
			field := pdu.FieldByName(sf)
			n := Len(Id("data"))
			if open {
				n = Min(Len(Id("data")), Lit(last))
			}
			g.Id("result").Dot(sf).Op("=").Add(qualType(*field, pdu)).Call(n)
		}
		fail := func() *Statement { return Return(zero, Id(errText)) }
		for _, grp := range sizeGroups(pdu) {
			dec := func(b *Group, field parse.Field) {
				emitLayoutFieldDecode(b, field, pdu, fail)
				if field.HasRange {
					b.If(rangeCond(field, Id("result").Dot(field.Name))).Block(Return(zero, fieldRangeErr(ev, pdu, field)))
				}
			}
			if grp.size == 0 {
				for _, field := range grp.fields {
					dec(g, field)
				}
				continue
			}
			g.If(Len(Id("data")).Op(">=").Lit(grp.size)).BlockFunc(func(b *Group) {
				for _, field := range grp.fields {
					dec(b, field)
				}
			})
		}
		g.Return(Id("result"), Nil())
	})
}

func emitLayoutFieldDecode(g *Group, field parse.Field, pdu parse.PDUStruct, textFail func() *Statement) {
	target := Id("result").Dot(field.Name)
	switch {
	case field.Kind == parse.FieldPacked && field.Encoding == parse.EncodingHex:
		a := field.BitStart / 8
		g.For(Id("_i").Op(":=").Lit(0), Id("_i").Op("<").Lit(field.ArrayLen), Id("_i").Op("++")).BlockFunc(func(b *Group) {
			b.Var().Id("_v").Byte()
			b.For(Id("_j").Op(":=").Lit(a).Op("+").Lit(2).Op("*").Id("_i"), Id("_j").Op("<").Lit(a+2).Op("+").Lit(2).Op("*").Id("_i"), Id("_j").Op("++")).Block(
				Id("_c").Op(":=").Id("data").Index(Id("_j")),
				Switch().Block(
					Case(Id("_c").Op(">=").LitRune('0').Op("&&").Id("_c").Op("<=").LitRune('9')).Block(Id("_v").Op("=").Id("_v").Op("<<").Lit(4).Op("|").Parens(Id("_c").Op("-").LitRune('0'))),
					Case(Id("_c").Op(">=").LitRune('A').Op("&&").Id("_c").Op("<=").LitRune('F')).Block(Id("_v").Op("=").Id("_v").Op("<<").Lit(4).Op("|").Parens(Id("_c").Op("-").LitRune('A').Op("+").Lit(10))),
					Case(Id("_c").Op(">=").LitRune('a').Op("&&").Id("_c").Op("<=").LitRune('f')).Block(Id("_v").Op("=").Id("_v").Op("<<").Lit(4).Op("|").Parens(Id("_c").Op("-").LitRune('a').Op("+").Lit(10))),
					Default().Block(textFail()),
				),
			)
			b.Add(target.Clone()).Index(Id("_i")).Op("=").Id("_v")
		})
		return
	case field.Kind == parse.FieldPacked && field.Swap16:
		a, n := field.BitStart/8, field.BitWidth/8
		g.For(Id("_i").Op(":=").Lit(0), Id("_i").Op("<").Lit(n), Id("_i").Op("+=").Lit(2)).Block(
			target.Clone().Index(Id("_i")).Op("=").Id("data").Index(Lit(a).Op("+").Id("_i").Op("+").Lit(1)),
			target.Clone().Index(Id("_i").Op("+").Lit(1)).Op("=").Id("data").Index(Lit(a).Op("+").Id("_i")),
		)
		return
	case isIntegerField(field) && (field.Encoding != parse.EncodingBits || field.Bias != 0):
		tmp := "_v" + field.Name
		g.Var().Id(tmp).Uint64()
		readLayoutInt(g, field, tmp, textFail)
		val := Id(tmp)
		if field.Bias != 0 {
			val = Id(tmp).Op("+").Id(strconv.FormatUint(field.Bias, 10))
		}
		g.Add(target).Op("=").Add(qualType(field, pdu)).Call(val)
		return
	}
	emitFieldDecode(g, field, pdu)
}

func readLayoutInt(g *Group, field parse.Field, tmp string, textFail func() *Statement) {
	a, n := field.BitStart/8, field.BitWidth/8
	switch field.Encoding {
	case parse.EncodingLE:
		var e *Statement
		for i := range n {
			term := Uint64().Call(Id("data").Index(Lit(a + i)))
			if i > 0 {
				term = term.Op("<<").Lit(8 * i)
			}
			if e == nil {
				e = term
			} else {
				e = e.Op("|").Add(term)
			}
		}
		g.Id(tmp).Op("=").Add(e)
	case parse.EncodingDecimal:
		loop := For(Id("_i").Op(":=").Lit(a), Id("_i").Op("<").Lit(a+n), Id("_i").Op("++")).Block(
			Id("_c").Op(":=").Id("data").Index(Id("_i")),
			If(Id("_c").Op("<").LitRune('0').Op("||").Id("_c").Op(">").LitRune('9')).Block(textFail()),
			Id(tmp).Op("=").Id(tmp).Op("*").Lit(10).Op("+").Uint64().Call(Id("_c").Op("-").LitRune('0')),
		)
		if !field.NulZero {
			g.Add(loop)
			break
		}
		var cond *Statement
		for i := range n {
			term := Id("data").Index(Lit(a + i)).Op("!=").Lit(0)
			if cond == nil {
				cond = term
			} else {
				cond = cond.Op("||").Add(term)
			}
		}
		g.If(cond).Block(loop)
	case parse.EncodingHex:
		g.For(Id("_i").Op(":=").Lit(a), Id("_i").Op("<").Lit(a+n), Id("_i").Op("++")).Block(
			Id("_c").Op(":=").Id("data").Index(Id("_i")),
			Switch().Block(
				Case(Id("_c").Op(">=").LitRune('0').Op("&&").Id("_c").Op("<=").LitRune('9')).Block(
					Id(tmp).Op("=").Id(tmp).Op("<<").Lit(4).Op("|").Uint64().Call(Id("_c").Op("-").LitRune('0'))),
				Case(Id("_c").Op(">=").LitRune('A').Op("&&").Id("_c").Op("<=").LitRune('F')).Block(
					Id(tmp).Op("=").Id(tmp).Op("<<").Lit(4).Op("|").Uint64().Call(Id("_c").Op("-").LitRune('A').Op("+").Lit(10))),
				Case(Id("_c").Op(">=").LitRune('a').Op("&&").Id("_c").Op("<=").LitRune('f')).Block(
					Id(tmp).Op("=").Id(tmp).Op("<<").Lit(4).Op("|").Uint64().Call(Id("_c").Op("-").LitRune('a').Op("+").Lit(10))),
				Default().Block(textFail()),
			),
		)
	case parse.EncodingBits:
		g.Id(tmp).Op("=").Add(readUint(field))
	}
}

func emitFrameEncode(f *File, pdu parse.PDUStruct, ev *errVars) {
	sizes := pdu.Frame.Sizes
	enc := encoder{pdu: pdu, bytes: sizes[len(sizes)-1], errs: ev, frame: true}
	def := pdu.Frame.Default
	doc := "AppendBinary appends the " + sizesText(sizes) + " of s to dst"
	if sf := pdu.Frame.SizeField; sf != "" {
		doc += ": " + sf + " selects the length, and 0 means " + strconv.Itoa(def)
	}
	doc += ". It returns dst unchanged and a *dmrgo.FieldRangeError when a field does not fit"
	if pdu.Frame.SizeField != "" {
		doc += ", or an error that wraps " + errLength + " when " + pdu.Frame.SizeField + " is not one of the lengths"
	}
	doc += ". It allocates only to grow dst."
	comment(f, doc+docCite(pdu))
	f.Func().Params(Id("s").Op("*").Id(pdu.Name)).Id("AppendBinary").Params(Id("dst").Index().Byte()).Params(Index().Byte(), Error()).BlockFunc(func(g *Group) {
		g.Id("_n").Op(":=").Len(Id("dst"))
		size := Lit(sizes[0])
		if sf := pdu.Frame.SizeField; sf != "" {
			g.Id("_size").Op(":=").Lit(def)
			g.Switch(Id("s").Dot(sf)).BlockFunc(func(sw *Group) {
				sw.Case(Lit(0), Lit(def))
				for _, s := range sizes {
					if s != def {
						sw.Case(Lit(s)).Block(Id("_size").Op("=").Lit(s))
					}
				}
				sw.Default().Block(Return(Id("dst"), ev.get("err"+pdu.Name+"Size", lengthError(pdu.Name+"."+sf+": not one of "+sizesText(sizes)))))
			})
			size = Id("_size")
		}
		g.For(Range().Add(size.Clone())).Block(Id("dst").Op("=").Append(Id("dst"), Lit(0)))
		g.Id("data").Op(":=").Id("dst").Index(Id("_n"), Empty())
		for _, m := range pdu.Magic {
			for i, b := range m.Bytes {
				g.Id("data").Index(Lit(m.Start/8 + i)).Op("=").Add(hexByte(b))
			}
		}
		for _, grp := range sizeGroups(pdu) {
			if grp.size == 0 {
				for _, field := range grp.fields {
					enc.layoutField(g, field)
				}
				continue
			}
			g.If(Id("_size").Op(">=").Lit(grp.size)).BlockFunc(func(b *Group) {
				for _, field := range grp.fields {
					enc.layoutField(b, field)
				}
			})
		}
		for i, c := range pdu.Consts {
			enc.constant(g, c, i)
		}
		g.Return(Id("dst"), Nil())
	})
}

func (e encoder) layoutField(g *Group, field parse.Field) {
	source := Id("s").Dot(field.Name)
	if field.HasRange {
		g.If(rangeCond(field, source)).Block(e.fail(fieldRangeErr(e.errs, e.pdu, field)))
	}
	switch {
	case field.Kind == parse.FieldPacked && field.Encoding == parse.EncodingHex:
		a := field.BitStart / 8
		g.For(Id("_i").Op(":=").Lit(0), Id("_i").Op("<").Lit(field.ArrayLen), Id("_i").Op("++")).Block(
			Id("data").Index(Lit(a).Op("+").Lit(2).Op("*").Id("_i")).Op("=").Lit("0123456789ABCDEF").Index(source.Clone().Index(Id("_i")).Op(">>").Lit(4)),
			Id("data").Index(Lit(a+1).Op("+").Lit(2).Op("*").Id("_i")).Op("=").Lit("0123456789ABCDEF").Index(source.Clone().Index(Id("_i")).Op("&").Lit(0xF)),
		)
		return
	case field.Kind == parse.FieldPacked && field.Swap16:
		a, n := field.BitStart/8, field.BitWidth/8
		g.For(Id("_i").Op(":=").Lit(0), Id("_i").Op("<").Lit(n), Id("_i").Op("+=").Lit(2)).Block(
			Id("data").Index(Lit(a).Op("+").Id("_i")).Op("=").Add(source.Clone()).Index(Id("_i").Op("+").Lit(1)),
			Id("data").Index(Lit(a).Op("+").Id("_i").Op("+").Lit(1)).Op("=").Add(source.Clone()).Index(Id("_i")),
		)
		return
	case isIntegerField(field) && (field.Encoding != parse.EncodingBits || field.Bias != 0):
		e.layoutInt(g, field, source)
		return
	}
	e.field(g, field)
}

func (e encoder) layoutInt(g *Group, field parse.Field, source *Statement) {
	tmp := "_v" + field.Name
	n := field.BitWidth / 8
	switch {
	case field.Bias != 0:
		hi := field.Bias + (uint64(1)<<field.BitWidth - 1)
		g.If(Uint64().Call(source.Clone()).Op("<").Id(strconv.FormatUint(field.Bias, 10)).Op("||").Uint64().Call(source.Clone()).Op(">").Id(strconv.FormatUint(hi, 10))).Block(
			e.failRange(field, Lit(float64(field.Bias)), Lit(float64(hi))),
		)
		g.Id(tmp).Op(":=").Uint64().Call(source.Clone()).Op("-").Id(strconv.FormatUint(field.Bias, 10))
	case field.Encoding == parse.EncodingDecimal:
		hi := math.Pow10(n) - 1
		g.If(Uint64().Call(source.Clone()).Op(">").Lit(uint64(hi))).Block(e.failRange(field, Lit(0.0), Lit(hi)))
		g.Id(tmp).Op(":=").Uint64().Call(source.Clone())
	default:
		w := field.DigitWidth()
		if needsDigitCheck(field, w) {
			g.If(Uint64().Call(source.Clone()).Op(">>").Lit(w).Op("!=").Lit(0)).Block(e.failRange(field, nil, nil))
		}
		g.Id(tmp).Op(":=").Uint64().Call(source.Clone())
	}
	writeLayoutInt(g, field, tmp)
}

func needsDigitCheck(field parse.Field, w int) bool {
	switch field.BaseType {
	case typUint8, "byte":
		return w < 8
	case typUint16:
		return w < 16
	case typUint32:
		return w < 32
	case typUint64:
		return w < 64
	}
	return true
}

func writeLayoutInt(g *Group, field parse.Field, tmp string) {
	a, n := field.BitStart/8, field.BitWidth/8
	switch field.Encoding {
	case parse.EncodingLE:
		for i := range n {
			v := Id(tmp)
			if i > 0 {
				v = Id(tmp).Op(">>").Lit(8 * i)
			}
			g.Id("data").Index(Lit(a + i)).Op("=").Byte().Call(v)
		}
	case parse.EncodingDecimal:
		g.For(Id("_i").Op(":=").Lit(a+n-1), Id("_i").Op(">=").Lit(a), Id("_i").Op("--")).Block(
			Id("data").Index(Id("_i")).Op("=").LitRune('0').Op("+").Byte().Call(Id(tmp).Op("%").Lit(10)),
			Id(tmp).Op("/=").Lit(10),
		)
	case parse.EncodingHex:
		g.For(Id("_i").Op(":=").Lit(a+n-1), Id("_i").Op(">=").Lit(a), Id("_i").Op("--")).Block(
			Id("data").Index(Id("_i")).Op("=").Lit("0123456789ABCDEF").Index(Id(tmp).Op("&").Lit(0xF)),
			Id(tmp).Op(">>=").Lit(4),
		)
	case parse.EncodingBits:
		storeBits(g, field.Ranges(), Id(tmp), tmp+"Bits")
	}
}

func viewDoc(field parse.Field, verb string) (string, error) {
	if field.Doc == "" {
		return "", fmt.Errorf("field %s of a dmr:view needs a doc comment", field.Name)
	}
	d := strings.Join(strings.Fields(field.Doc), " ")
	rest, ok := strings.CutPrefix(d, field.Name+" is ")
	if !ok {
		return "", fmt.Errorf("doc comment of view field %s must start with %q", field.Name, field.Name+" is ")
	}
	return verb + " " + rest, nil
}

func emitView(f *File, pdu parse.PDUStruct, ev *errVars) error {
	typ := pdu.View.Type
	recv := func() *Statement { return Id("c").Op("*").Id(typ) }
	pdu.Name = typ
	enc := encoder{pdu: pdu, bytes: pdu.View.Size, errs: ev, view: true}
	local := func(g *Group, field parse.Field) {
		g.Var().Id("result").Struct(Id(field.Name).Add(qualType(field, pdu)))
	}
	for _, field := range pdu.Fields {
		if field.Skip {
			continue
		}
		getDoc, err := viewDoc(field, "returns")
		if err != nil {
			return err
		}
		setDoc, _ := viewDoc(field, "sets")
		a, n := field.BitStart/8, field.BitWidth/8
		switch {
		case field.Kind == parse.FieldPacked && field.Text:
			comment(f, field.Name+" "+getDoc+" The text has its trailing spaces and NUL octets removed, and points into c.")
			f.Func().Params(recv()).Id(field.Name).Params().Index().Byte().Block(
				Id("_e").Op(":=").Lit(a+n),
				For(Id("_e").Op(">").Lit(a).Op("&&").Parens(Id("c").Index(Id("_e").Op("-").Lit(1)).Op("==").LitRune(' ').Op("||").Id("c").Index(Id("_e").Op("-").Lit(1)).Op("==").Lit(0))).Block(Id("_e").Op("--")),
				Return(Id("c").Index(Lit(a), Id("_e"))),
			)
			f.Line()
			comment(f, "Set"+field.Name+" "+setDoc+" It pads v with spaces, and returns a *dmrgo.FieldRangeError, and changes nothing, when v is longer than "+strconv.Itoa(n)+" octets.")
			f.Func().Params(recv()).Id("Set"+field.Name).Params(Id("v").Index().Byte()).Error().Block(
				If(Len(Id("v")).Op(">").Lit(n)).Block(Return(ev.get("err"+pdu.Name+field.Name+"Range", Op("&").Qual(rootPkg, "FieldRangeError").Values(Dict{Id("Type"): Lit(typ), Id("Field"): Lit(field.Name), Id("Bits"): Lit(8 * n)})))),
				Id("_k").Op(":=").Copy(Id("c").Index(Lit(a), Lit(a+n)), Id("v")),
				For(Id("_i").Op(":=").Lit(a).Op("+").Id("_k"), Id("_i").Op("<").Lit(a+n), Id("_i").Op("++")).Block(Id("c").Index(Id("_i")).Op("=").LitRune(' ')),
				Return(Nil()),
			)
		case field.Kind == parse.FieldPacked && field.Encoding == parse.EncodingHex:
			return fmt.Errorf("field %s: hex packed fields are not supported in a dmr:view", field.Name)
		case field.Kind == parse.FieldPacked, field.Kind == parse.FieldRaw:
			comment(f, field.Name+" "+getDoc)
			f.Func().Params(recv()).Id(field.Name).Params().Add(qualType(field, pdu)).BlockFunc(func(g *Group) {
				local(g, field)
				g.Id("data").Op(":=").Id("c").Index(Empty(), Empty())
				emitLayoutFieldDecode(g, field, pdu, nil)
				g.Return(Id("result").Dot(field.Name))
			})
			f.Line()
			comment(f, "Set"+field.Name+" "+setDoc)
			f.Func().Params(recv()).Id("Set" + field.Name).Params(Id("v").Add(qualType(field, pdu))).BlockFunc(func(g *Group) {
				g.Id("data").Op(":=").Id("c").Index(Empty(), Empty())
				g.Id("s").Op(":=").Struct(Id(field.Name).Add(qualType(field, pdu))).Values(Id("v"))
				enc.layoutField(g, field)
			})
		default:
			text := field.Encoding == parse.EncodingDecimal || field.Encoding == parse.EncodingHex
			ret := []Code{qualType(field, pdu)}
			extra := ""
			if text {
				ret = append(ret, Error())
				extra = " It returns " + errText + " when the field holds a character that is not a digit."
			}
			comment(f, field.Name+" "+getDoc+extra)
			f.Func().Params(recv()).Id(field.Name).Params().Params(ret...).BlockFunc(func(g *Group) {
				local(g, field)
				g.Id("data").Op(":=").Id("c").Index(Empty(), Empty())
				emitLayoutFieldDecode(g, field, pdu, func() *Statement {
					return Return(Lit(0), Id(errText))
				})
				if text {
					g.Return(Id("result").Dot(field.Name), Nil())
				} else {
					g.Return(Id("result").Dot(field.Name))
				}
			})
			f.Line()
			canFail := field.Bias != 0 || field.Encoding == parse.EncodingDecimal || needsDigitCheck(field, field.DigitWidth())
			if field.Kind == parse.FieldBool {
				canFail = false
			}
			if canFail {
				comment(f, "Set"+field.Name+" "+setDoc+" It returns a *dmrgo.FieldRangeError, and changes nothing, when v does not fit.")
			} else {
				comment(f, "Set"+field.Name+" "+setDoc)
			}
			fn := f.Func().Params(recv()).Id("Set" + field.Name).Params(Id("v").Add(qualType(field, pdu)))
			if canFail {
				fn = fn.Error()
			}
			fn.BlockFunc(func(g *Group) {
				g.Id("data").Op(":=").Id("c").Index(Empty(), Empty())
				g.Id("s").Op(":=").Struct(Id(field.Name).Add(qualType(field, pdu))).Values(Id("v"))
				enc.layoutField(g, field)
				if canFail {
					g.Return(Nil())
				}
			})
		}
		f.Line()
	}
	f.Comment("Reset sets c to an empty " + typ + ": the fixed octets, spaces in the text")
	f.Comment("fields and zero in the number fields.")
	f.Func().Params(recv()).Id("Reset").Params().BlockFunc(func(g *Group) {
		g.Id("data").Op(":=").Id("c").Index(Empty(), Empty())
		g.Clear(Id("data"))
		for _, m := range pdu.Magic {
			for i, b := range m.Bytes {
				g.Id("data").Index(Lit(m.Start/8 + i)).Op("=").Add(hexByte(b))
			}
		}
		for _, field := range pdu.Fields {
			if field.Skip {
				continue
			}
			a, n := field.BitStart/8, field.BitWidth/8
			var fill Code
			switch {
			case field.Text:
				fill = LitRune(' ')
			case field.Encoding == parse.EncodingDecimal || field.Encoding == parse.EncodingHex:
				fill = LitRune('0')
			default:
				continue
			}
			g.For(Id("_i").Op(":=").Lit(a), Id("_i").Op("<").Lit(a+n), Id("_i").Op("++")).Block(Id("data").Index(Id("_i")).Op("=").Add(fill))
		}
	})
	if len(pdu.Magic) > 0 {
		f.Line()
		f.Func().Params(recv()).Id("magicOK").Params().Bool().BlockFunc(func(g *Group) {
			var cond *Statement
			for _, m := range pdu.Magic {
				for i, b := range m.Bytes {
					c := Id("c").Index(Lit(m.Start/8 + i)).Op("==").Add(hexByte(b))
					if cond == nil {
						cond = c
					} else {
						cond = cond.Op("&&").Add(c)
					}
				}
			}
			g.Return(cond)
		})
	}
	return nil
}
