package emit

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
	. "github.com/dave/jennifer/jen"
)

const (
	wirePkg     = "github.com/USA-RedDragon/dmrgo/v2/internal/wire"
	errToken    = "ErrToken"
	errVarint   = "ErrVarint"
	streamLocal = "result"
)

type streamGen struct {
	pdu   parse.PDUStruct
	types map[string]parse.PDUStruct
	errs  *errVars
}

func (s streamGen) fail(name string) *Statement {
	return Return(Lit(0), Id(name))
}

func (s streamGen) failStatus() *Statement {
	return If(Id("_st").Op("!=").Qual(wirePkg, "OK")).Block(
		If(Id("_st").Op("==").Qual(wirePkg, "Short")).Block(s.fail(errShort)),
		s.fail(errVarint),
	)
}

func (s streamGen) nested(f parse.StreamField) (parse.PDUStruct, error) {
	p, ok := s.types[f.TypeName]
	if !ok {
		return p, fmt.Errorf("field %s: type %s must be a dmrgen struct of the same file", f.Name, f.TypeName)
	}
	switch f.SKind {
	case parse.StreamFrame:
		if p.Frame == nil || len(p.Frame.Sizes) != 1 || p.Frame.Open {
			return p, fmt.Errorf("field %s: %s must be a dmr:frame of one size", f.Name, f.TypeName)
		}
	case parse.StreamNested:
		if p.Stream == nil || p.Stream.Tokens {
			return p, fmt.Errorf("field %s: %s must be a dmr:stream", f.Name, f.TypeName)
		}
	case parse.StreamTokens:
		if p.Stream == nil || !p.Stream.Tokens {
			return p, fmt.Errorf("field %s: %s must be a dmr:tokens struct", f.Name, f.TypeName)
		}
	case parse.StreamInt, parse.StreamFixed, parse.StreamUintvar, parse.StreamSintvar, parse.StreamUfloatvar,
		parse.StreamSfloatvar, parse.StreamArray, parse.StreamBytes, parse.StreamFlag:
	}
	return p, nil
}

func pathOf(root, path string) *Statement {
	st := Id(root)
	for _, p := range strings.Split(path, ".") {
		st = st.Dot(p)
	}
	return st
}

func (s streamGen) constant(v string) *Statement {
	if _, err := strconv.ParseInt(v, 0, 64); err == nil {
		return Id(v)
	}
	if pkg, name, ok := strings.Cut(v, "."); ok {
		return Qual(resolveImportPath(pkg, s.pdu.SourceFile), name)
	}
	return Id(v)
}

func (s streamGen) cond(root string, c *parse.Cond) *Statement {
	if len(c.And) == 0 {
		return s.term(root, c)
	}
	e := Parens(s.term(root, c))
	for i := range c.And {
		e = e.Op("&&").Parens(s.term(root, &c.And[i]))
	}
	return e
}

func (s streamGen) term(root string, c *parse.Cond) *Statement {
	p := pathOf(root, c.Path)
	if c.Op == "" {
		return p
	}
	join := "||"
	if c.Op == "!=" {
		join = "&&"
	}
	var e *Statement
	for _, v := range c.Values {
		t := p.Clone().Op(c.Op).Add(s.constant(v))
		if e == nil {
			e = t
		} else {
			e = e.Op(join).Add(t)
		}
	}
	return e
}

func readInt(n int, le bool, at func(i int) *Statement) *Statement {
	var e *Statement
	for i := range n {
		shift := 8 * (n - 1 - i)
		if le {
			shift = 8 * i
		}
		term := Uint64().Call(at(i))
		if shift > 0 {
			term = term.Op("<<").Lit(shift)
		}
		if e == nil {
			e = term
		} else {
			e = e.Op("|").Add(term)
		}
	}
	return e
}

func dataAt(i int) *Statement {
	if i == 0 {
		return Id("data").Index(Id("_o"))
	}
	return Id("data").Index(Id("_o").Op("+").Lit(i))
}

func signedValue(n int, v *Statement) *Statement {
	if n == 8 {
		return Int64().Call(v)
	}
	sh := 64 - 8*n
	return Int64().Call(v.Op("<<").Lit(sh)).Op(">>").Lit(sh)
}

func scaleText(f parse.StreamField) string {
	return strconv.FormatUint(f.ScaleNum, 10) + ".0 / (1 << " + strconv.Itoa(f.ScaleExp) + ")"
}

func (s streamGen) cast(f parse.StreamField, v Code) *Statement {
	return qualType(f.Field, s.pdu).Call(v)
}

func (s streamGen) decodeValue(g *Group, f parse.StreamField, target *Statement) error {
	tmp := "_v" + f.Name
	switch f.SKind {
	case parse.StreamInt, parse.StreamFixed:
		g.If(Len(Id("data")).Op("-").Id("_o").Op("<").Lit(f.Octets)).Block(s.fail(errShort))
		raw := readInt(f.Octets, f.LE, dataAt)
		var v *Statement
		switch {
		case f.SKind == parse.StreamFixed && f.SignMag:
			sign := 8*f.Octets - 1
			g.Id(tmp).Op(":=").Add(raw)
			g.Add(target.Clone()).Op("=").Float64().Call(Id(tmp).Op("&").Lit(uint64(1)<<sign - 1)).Op("*").Parens(Id(scaleText(f)))
			g.If(Id(tmp).Op(">>").Lit(sign).Op("!=").Lit(0)).Block(target.Clone().Op("=").Op("-").Add(target.Clone()))
			g.Id("_o").Op("+=").Lit(f.Octets)
			return nil
		case f.SKind == parse.StreamFixed && f.SignedInt:
			v = Float64().Call(signedValue(f.Octets, Parens(raw))).Op("*").Parens(Id(scaleText(f)))
		case f.SKind == parse.StreamFixed:
			v = Float64().Call(raw).Op("*").Parens(Id(scaleText(f)))
		case f.SignedInt:
			v = s.cast(f, signedValue(f.Octets, Parens(raw)))
		default:
			v = s.cast(f, raw)
		}
		g.Add(target.Clone()).Op("=").Add(v)
		g.Id("_o").Op("+=").Lit(f.Octets)
		if f.Const != "" {
			g.If(target.Clone().Op("!=").Add(s.constant(f.Const))).Block(s.fail(errSignature))
		}
	case parse.StreamUintvar:
		g.List(Id(tmp), Id("_on"), Id("_st")).Op(":=").Qual(wirePkg, "Uintvar").Call(Id("data"), Id("_o"))
		g.Add(s.failStatus())
		if w := basicWidth(f); w > 0 && w < 64 {
			g.If(Id(tmp).Op(">>").Lit(w).Op("!=").Lit(0)).Block(s.fail(errVarint))
		}
		g.Add(target.Clone()).Op("=").Add(s.cast(f, Id(tmp)))
		g.Id("_o").Op("=").Id("_on")
	case parse.StreamSintvar:
		g.List(Id(tmp), Id("_"), Id("_on"), Id("_st")).Op(":=").Qual(wirePkg, "Sintvar").Call(Id("data"), Id("_o"))
		g.Add(s.failStatus())
		if w := basicWidth(f); w > 0 && w < 64 {
			lo, hi := -(int64(1) << (w - 1)), int64(1)<<(w-1)-1
			g.If(Id(tmp).Op("<").Lit(lo).Op("||").Id(tmp).Op(">").Lit(hi)).Block(s.fail(errVarint))
		}
		g.Add(target.Clone()).Op("=").Add(s.cast(f, Id(tmp)))
		g.Id("_o").Op("=").Id("_on")
	case parse.StreamUfloatvar, parse.StreamSfloatvar:
		fn := "Ufloatvar"
		if f.SKind == parse.StreamSfloatvar {
			fn = "Sfloatvar"
		}
		g.List(Id(tmp), Id("_on"), Id("_st")).Op(":=").Qual(wirePkg, fn).Call(Id("data"), Id("_o"))
		g.Add(s.failStatus())
		g.Add(target.Clone()).Op("=").Id(tmp)
		g.Id("_o").Op("=").Id("_on")
	case parse.StreamFrame:
		p, err := s.nested(f)
		if err != nil {
			return err
		}
		n := p.Frame.Sizes[0]
		g.If(Len(Id("data")).Op("-").Id("_o").Op("<").Lit(n)).Block(s.fail(errShort))
		g.List(Id(tmp), Id("_err")).Op(":=").Id(p.DecodeFuncName(p.Name)).Call(Id("data").Index(Id("_o"), Id("_o").Op("+").Lit(n)))
		g.If(Id("_err").Op("!=").Nil()).Block(Return(Lit(0), Id("_err")))
		g.Add(target.Clone()).Op("=").Id(tmp)
		g.Id("_o").Op("+=").Lit(n)
	case parse.StreamNested, parse.StreamTokens:
		p, err := s.nested(f)
		if err != nil {
			return err
		}
		g.List(Id("_on"+f.Name), Id("_err")).Op(":=").Id(atName(p.Name)).Call(Op("&").Add(target.Clone()), Id("data"), Id("_o"))
		g.If(Id("_err").Op("!=").Nil()).Block(Return(Lit(0), Id("_err")))
		g.Id("_o").Op("=").Id("_on" + f.Name)
	case parse.StreamArray:
		g.If(Len(Id("data")).Op("-").Id("_o").Op("<").Lit(f.Octets)).Block(s.fail(errShort))
		g.Copy(target.Clone().Index(Empty(), Empty()), Id("data").Index(Id("_o"), Id("_o").Op("+").Lit(f.Octets)))
		g.Id("_o").Op("+=").Lit(f.Octets)
	case parse.StreamBytes:
		return s.decodeBytes(g, f, target)
	case parse.StreamFlag:
		g.Add(target.Clone()).Op("=").True()
	}
	return nil
}

func (s streamGen) decodeBytes(g *Group, f parse.StreamField, target *Statement) error {
	ln := "_l" + f.Name
	switch f.Length {
	case parse.LengthRest:
		g.Add(target.Clone()).Op("=").Id("data").Index(Id("_o"), Len(Id("data")))
		g.Id("_o").Op("=").Len(Id("data"))
		return nil
	case parse.LengthPrefix:
		if f.VarLen {
			g.List(Id("_vl"+f.Name), Id("_on"), Id("_st")).Op(":=").Qual(wirePkg, "Uintvar").Call(Id("data"), Id("_o"))
			g.Add(s.failStatus())
			g.Id("_o").Op("=").Id("_on")
			g.If(Id("_vl" + f.Name).Op(">").Uint64().Call(Len(Id("data")).Op("-").Id("_o"))).Block(s.fail(errShort))
			g.Id(ln).Op(":=").Int().Call(Id("_vl" + f.Name))
		} else {
			g.If(Len(Id("data")).Op("-").Id("_o").Op("<").Lit(f.Octets)).Block(s.fail(errShort))
			g.Id(ln).Op(":=").Int().Call(readInt(f.Octets, false, dataAt))
			g.Id("_o").Op("+=").Lit(f.Octets)
		}
	case parse.LengthExpr:
		e := Int().Call(pathOf(streamLocal, f.Expr.Path))
		if f.Expr.Mul != 1 {
			e = e.Op("*").Lit(f.Expr.Mul)
		}
		e = addOffset(e, f.Expr.Add)
		g.Id(ln).Op(":=").Add(e)
		g.If(Id(ln).Op("<").Lit(0)).Block(s.fail(errLength))
	}
	g.If(Len(Id("data")).Op("-").Id("_o").Op("<").Id(ln)).Block(s.fail(errShort))
	g.Add(target.Clone()).Op("=").Id("data").Index(Id("_o"), Id("_o").Op("+").Id(ln))
	g.Id("_o").Op("+=").Id(ln)
	return nil
}

func addOffset(e *Statement, add int) *Statement {
	switch {
	case add > 0:
		return e.Op("+").Lit(add)
	case add < 0:
		return e.Op("-").Lit(-add)
	}
	return e
}

func basicWidth(f parse.StreamField) int {
	switch f.BaseType {
	case typUint8, "int8", typByte:
		return 8
	case typUint16, "int16":
		return 16
	case typUint32, "int32":
		return 32
	}
	return 0
}

func atName(typ string) string {
	return "decode" + strings.ToUpper(typ[:1]) + typ[1:] + "At"
}

func (s streamGen) boundPatch(field string) (frame parse.PDUStruct, sub parse.Field, err error) {
	head, rest, _ := strings.Cut(s.pdu.Stream.BoundPath, ".")
	for _, f := range s.pdu.Stream.Fields {
		if f.Name != head {
			continue
		}
		frame, err = s.nested(f)
		if err != nil {
			return frame, sub, err
		}
		g := frame.FieldByName(rest)
		if g == nil || g.BitStart%8 != 0 || g.BitWidth%8 != 0 || len(g.ExtraBitRanges) > 0 || g.Encoding != parse.EncodingBits || g.Bias != 0 {
			return frame, sub, fmt.Errorf("dmr:bound %s must be a big-endian integer of whole octets", s.pdu.Stream.BoundPath)
		}
		return frame, *g, nil
	}
	return frame, sub, fmt.Errorf("dmr:bound: no field %s", field)
}

func (s streamGen) emitBound(g *Group, mode parse.BoundMode, v *Statement) {
	end := Id("_o").Op("+").Int().Call(v)
	if mode == parse.BoundTotal {
		end = Id("_s").Op("+").Int().Call(v.Clone())
	}
	g.Id("_end").Op(":=").Add(end)
	g.If(Id("_end").Op("<").Id("_o")).Block(s.fail(errLength))
	g.If(Id("_end").Op(">").Len(Id("data"))).Block(s.fail(errShort))
	g.Id("data").Op("=").Id("data").Index(Empty(), Id("_end"))
}

func (s streamGen) emitDecodeAt(f *File) error {
	pdu := s.pdu
	st := pdu.Stream
	var err error
	f.Func().Id(atName(pdu.Name)).Params(Id(streamLocal).Op("*").Id(pdu.Name), Id("data").Index().Byte(), Id("_o").Int()).Params(Int(), Error()).BlockFunc(func(g *Group) {
		if st.Tokens {
			g.For(Id("_o").Op("<").Len(Id("data"))).BlockFunc(func(lp *Group) {
				lp.Id("_t").Op(":=").Id("data").Index(Id("_o"))
				lp.Id("_o").Op("++")
				lp.Switch(Id("_t")).BlockFunc(func(sw *Group) {
					for _, fl := range st.Fields {
						if fl.Skip {
							continue
						}
						sw.Case(hexByte(fl.Token)).BlockFunc(func(c *Group) {
							if e := s.decodeValue(c, fl, Id(streamLocal).Dot(fl.Name)); e != nil {
								err = e
							}
							if fl.Has != "" {
								c.Id(streamLocal).Dot(fl.Has).Op("=").True()
							}
						})
					}
					sw.Default().Block(s.fail(errToken))
				})
			})
			g.Return(Id("_o"), Nil())
			return
		}
		usesStart := st.BoundMode == parse.BoundTotal
		for _, fl := range st.Fields {
			if fl.Bound == parse.BoundTotal {
				usesStart = true
			}
		}
		if usesStart {
			g.Id("_s").Op(":=").Id("_o")
		}
		bounded := false
		for _, fl := range st.Fields {
			if fl.Skip {
				continue
			}
			target := Id(streamLocal).Dot(fl.Name)
			emitOne := func(b *Group) {
				if e := s.decodeValue(b, fl, target); e != nil {
					err = e
				}
				if fl.Bound != parse.BoundNone {
					s.emitBound(b, fl.Bound, target.Clone())
					bounded = true
				}
				if st.BoundPath != "" {
					if head, _, _ := strings.Cut(st.BoundPath, "."); head == fl.Name {
						s.emitBound(b, st.BoundMode, pathOf(streamLocal, st.BoundPath))
						bounded = true
					}
				}
			}
			if fl.If != nil {
				g.If(s.cond(streamLocal, fl.If)).BlockFunc(emitOne)
				continue
			}
			emitOne(g)
		}
		if bounded {
			g.If(Id("_o").Op("!=").Len(Id("data"))).Block(s.fail(errLength))
		}
		g.Return(Id("_o"), Nil())
	})
	return err
}

func streamConds(pdu parse.PDUStruct) []string {
	conds := []string{errShort + " when data ends inside a field"}
	hasLen := pdu.Stream.BoundPath != ""
	consts, tokens, varints := false, pdu.Stream.Tokens, false
	for _, f := range pdu.Stream.Fields {
		switch {
		case f.Bound != parse.BoundNone, f.Length == parse.LengthExpr && f.SKind == parse.StreamBytes:
			hasLen = true
		}
		if f.Const != "" {
			consts = true
		}
		switch f.SKind {
		case parse.StreamUintvar, parse.StreamSintvar, parse.StreamUfloatvar, parse.StreamSfloatvar:
			varints = true
		case parse.StreamBytes:
			if f.VarLen {
				varints = true
			}
		case parse.StreamInt, parse.StreamFixed, parse.StreamFrame, parse.StreamNested, parse.StreamTokens,
			parse.StreamArray, parse.StreamFlag:
		}
	}
	fs := pdu.Stream.Fields
	rest := len(fs) > 0 && fs[len(fs)-1].SKind == parse.StreamBytes && fs[len(fs)-1].Length == parse.LengthRest && fs[len(fs)-1].If == nil
	if hasLen || !pdu.Stream.Trailing && !rest {
		conds = append(conds, errLength+" when a length does not match the data")
	}
	if consts {
		conds = append(conds, errSignature+" when a fixed field does not hold its value")
	}
	if tokens {
		conds = append(conds, errToken+" for an unknown token")
	}
	if varints {
		conds = append(conds, errVarint+" for a number that does not fit its field")
	}
	return conds
}

func joinConds(c []string) string {
	if len(c) == 1 {
		return c[0]
	}
	return strings.Join(c[:len(c)-1], ", ") + " and " + c[len(c)-1]
}

func emitStreamDecode(f *File, pdu parse.PDUStruct, types map[string]parse.PDUStruct, ev *errVars) error {
	s := streamGen{pdu: pdu, types: types, errs: ev}
	if !pdu.UnexportedDecode[pdu.Name] {
		name := pdu.DecodeFuncName(pdu.Name)
		doc := name + " decodes " + docObject(pdu)
		doc = strings.TrimSuffix(doc, ".") + " from data. It returns " + joinConds(streamConds(pdu)) +
			". It does not copy: the byte string fields point into data."
		if pdu.Stream.Trailing {
			doc += " It ignores octets after the length that the message gives."
		}
		if pdu.Stream.DecodeDoc != "" {
			doc += " " + pdu.Stream.DecodeDoc
		}
		if pdu.Stream.Into {
			comment(f, name+"Into decodes "+strings.TrimPrefix(strings.TrimPrefix(doc, name+" decodes "), "")+" into m. On an error m is the zero "+pdu.Name+". It returns dmrgo.ErrNilArgument for a nil m."+docCite(pdu))
			f.Func().Id(name+"Into").Params(Id("m").Op("*").Id(pdu.Name), Id("data").Index().Byte()).Error().BlockFunc(func(g *Group) {
				g.If(Id("m").Op("==").Nil()).Block(Return(Qual(rootPkg, "ErrNilArgument")))
				g.Op("*").Id("m").Op("=").Id(pdu.Name).Values()
				g.List(Id("n"), Id("err")).Op(":=").Id(atName(pdu.Name)).Call(Id("m"), Id("data"), Lit(0))
				if !pdu.Stream.Trailing {
					g.If(Id("err").Op("==").Nil().Op("&&").Id("n").Op("!=").Len(Id("data"))).Block(Id("err").Op("=").Id(errLength))
				} else {
					g.Id("_").Op("=").Id("n")
				}
				g.If(Id("err").Op("!=").Nil()).Block(Op("*").Id("m").Op("=").Id(pdu.Name).Values())
				g.Return(Id("err"))
			})
			f.Line()
			return s.emitDecodeAt(f)
		}
		comment(f, doc+docCite(pdu))
		f.Func().Id(name).Params(Id("data").Index().Byte()).Params(Id(pdu.Name), Error()).BlockFunc(func(g *Group) {
			g.Var().Id("v").Id(pdu.Name)
			n := Id("n")
			if pdu.Stream.Trailing {
				n = Id("_")
			}
			g.List(n, Id("err")).Op(":=").Id(atName(pdu.Name)).Call(Op("&").Id("v"), Id("data"), Lit(0))
			g.If(Id("err").Op("!=").Nil()).Block(Return(Id(pdu.Name).Values(), Id("err")))
			if !pdu.Stream.Trailing {
				g.If(Id("n").Op("!=").Len(Id("data"))).Block(Return(Id(pdu.Name).Values(), Id(errLength)))
			}
			g.Return(Id("v"), Nil())
		})
		f.Line()
	}
	return s.emitDecodeAt(f)
}

type streamEnc struct {
	streamGen
}

func (e streamEnc) fail(err Code) *Statement {
	return Return(Id("dst").Index(Empty(), Id("_n")), err)
}

func (e streamEnc) rangeErr(f parse.StreamField, bits int, signed bool, lo, hi *float64) *Statement {
	values := Dict{Id("Type"): Lit(e.pdu.Name), Id("Field"): Lit(f.Name), Id("Bits"): Lit(bits)}
	if signed {
		values[Id("Signed")] = True()
	}
	if lo != nil {
		values[Id("Min")], values[Id("Max")] = Lit(*lo), Lit(*hi)
	}
	return e.fail(e.errs.get("err"+e.pdu.Name+strings.ReplaceAll(f.Name, ".", "")+"Range", Op("&").Qual(rootPkg, "FieldRangeError").Values(values)))
}

func appendInt(g *Group, n int, le bool, v *Statement) {
	args := make([]Code, 0, n)
	for i := range n {
		shift := 8 * (n - 1 - i)
		if le {
			shift = 8 * i
		}
		x := v.Clone()
		if shift > 0 {
			x = x.Op(">>").Lit(shift)
		}
		args = append(args, Byte().Call(x))
	}
	g.Id("dst").Op("=").Append(append([]Code{Id("dst")}, args...)...)
}

func (e streamEnc) encodeValue(g *Group, f parse.StreamField, src *Statement) error {
	tmp := "_v" + f.Name
	switch f.SKind {
	case parse.StreamInt:
		val := src.Clone()
		if f.Const != "" {
			val = e.constant(f.Const)
		}
		w := 8 * f.Octets
		if f.SignedInt {
			g.Id(tmp).Op(":=").Int64().Call(val)
			if bw := basicWidth(f); bw == 0 || bw > w {
				lo, hi := -(int64(1) << (w - 1)), int64(1)<<(w-1)-1
				g.If(Id(tmp).Op("<").Lit(lo).Op("||").Id(tmp).Op(">").Lit(hi)).Block(e.rangeErr(f, w, true, nil, nil))
			}
			appendInt(g, f.Octets, f.LE, Uint64().Call(Id(tmp)))
			return nil
		}
		g.Id(tmp).Op(":=").Uint64().Call(val)
		if bw := basicWidth(f); w < 64 && (bw == 0 || bw > w) {
			g.If(Id(tmp).Op(">>").Lit(w).Op("!=").Lit(0)).Block(e.rangeErr(f, w, false, nil, nil))
		}
		appendInt(g, f.Octets, f.LE, Id(tmp))
	case parse.StreamFixed:
		w := 8 * f.Octets
		if f.SignMag {
			hiRaw := math.Ldexp(1, w-1) - 1
			scale := float64(f.ScaleNum) / math.Ldexp(1, f.ScaleExp)
			lo, hi := -(hiRaw+1)*scale, (hiRaw+1)*scale
			g.Id(tmp).Op(":=").Qual("math", "Round").Call(Qual("math", "Abs").Call(src.Clone()).Op("/").Parens(Id(scaleText(f))))
			g.If(Op("!").Parens(Id(tmp).Op("<=").Lit(hiRaw + 1))).Block(e.rangeErr(f, w, true, &lo, &hi))
			raw := tmp + "Raw"
			g.Id(raw).Op(":=").Uint64().Call(Min(Id(tmp), Lit(hiRaw)))
			g.If(src.Clone().Op("<").Lit(0).Op("&&").Id(raw).Op("!=").Lit(0)).Block(Id(raw).Op("|=").Lit(uint64(1) << (w - 1)))
			appendInt(g, f.Octets, f.LE, Id(raw))
			return nil
		}
		var loRaw, hiRaw float64
		if f.SignedInt {
			loRaw, hiRaw = -math.Ldexp(1, w-1), math.Ldexp(1, w-1)-1
		} else {
			loRaw, hiRaw = 0, math.Ldexp(1, w)-1
		}
		scale := float64(f.ScaleNum) / math.Ldexp(1, f.ScaleExp)
		lo, hi := loRaw*scale, hiRaw*scale
		g.Id(tmp).Op(":=").Qual("math", "Round").Call(src.Clone().Op("/").Parens(Id(scaleText(f))))
		if f.Wrap {
			hi = (hiRaw + 1) * scale
			g.If(Id(tmp).Op("==").Lit(hiRaw + 1)).Block(Id(tmp).Op("=").Lit(loRaw))
		}
		g.If(Op("!").Parens(Id(tmp).Op(">=").Lit(loRaw).Op("&&").Id(tmp).Op("<=").Lit(hiRaw))).Block(e.rangeErr(f, w, f.SignedInt, &lo, &hi))
		if f.SignedInt {
			appendInt(g, f.Octets, f.LE, Uint64().Call(Int64().Call(Id(tmp))))
		} else {
			appendInt(g, f.Octets, f.LE, Uint64().Call(Id(tmp)))
		}
	case parse.StreamUintvar:
		g.Id("dst").Op("=").Qual(wirePkg, "AppendUintvar").Call(Id("dst"), Uint64().Call(src.Clone()))
	case parse.StreamSintvar:
		g.Id("dst").Op("=").Qual(wirePkg, "AppendSintvar").Call(Id("dst"), Int64().Call(src.Clone()), False())
	case parse.StreamUfloatvar, parse.StreamSfloatvar:
		fn, signed := "AppendUfloatvar", false
		if f.SKind == parse.StreamSfloatvar {
			fn, signed = "AppendSfloatvar", true
		}
		g.If(Op("!").Qual(wirePkg, "FloatvarFits").Call(src.Clone(), Lit(signed))).Block(e.rangeErr(f, 64, signed, nil, nil))
		g.Id("dst").Op("=").Qual(wirePkg, fn).Call(Id("dst"), src.Clone(), Lit(f.Prec))
	case parse.StreamFrame, parse.StreamNested, parse.StreamTokens:
		if _, err := e.nested(f); err != nil {
			return err
		}
		ev := "_err" + f.Name
		g.Var().Id(ev).Error()
		g.List(Id("dst"), Id(ev)).Op("=").Add(src.Clone()).Dot("AppendBinary").Call(Id("dst"))
		g.If(Id(ev).Op("!=").Nil()).Block(Return(Id("dst").Index(Empty(), Id("_n")), Id(ev)))
	case parse.StreamArray:
		g.Id("dst").Op("=").Append(Id("dst"), src.Clone().Index(Empty(), Empty()).Op("..."))
	case parse.StreamBytes:
		return e.encodeBytes(g, f, src)
	case parse.StreamFlag:
	}
	return nil
}

func (e streamEnc) encodeBytes(g *Group, f parse.StreamField, src *Statement) error {
	switch f.Length {
	case parse.LengthPrefix:
		switch {
		case f.VarLen:
			g.Id("dst").Op("=").Qual(wirePkg, "AppendUintvar").Call(Id("dst"), Uint64().Call(Len(src.Clone())))
		default:
			w := 8 * f.Octets
			g.If(Len(src.Clone()).Op(">>").Lit(w).Op("!=").Lit(0)).Block(e.rangeErr(f, w, false, nil, nil))
			appendInt(g, f.Octets, false, Len(src.Clone()))
		}
	case parse.LengthExpr:
		ex := Int().Call(pathOf("s", f.Expr.Path))
		if f.Expr.Mul != 1 {
			ex = ex.Op("*").Lit(f.Expr.Mul)
		}
		ex = addOffset(ex, f.Expr.Add)
		msg := e.pdu.Name + "." + f.Name + ": length does not match " + f.Expr.Path
		g.If(Len(src.Clone()).Op("!=").Add(ex)).Block(e.fail(e.errs.get("err"+e.pdu.Name+f.Name+"Length", lengthError(msg))))
	case parse.LengthRest:
	}
	g.Id("dst").Op("=").Append(Id("dst"), src.Clone().Op("..."))
	return nil
}

func emitStreamEncode(f *File, pdu parse.PDUStruct, types map[string]parse.PDUStruct, ev *errVars) error {
	e := streamEnc{streamGen{pdu: pdu, types: types, errs: ev}}
	st := pdu.Stream
	doc := "AppendBinary appends s to dst"
	var boundField *parse.StreamField
	for i := range st.Fields {
		if st.Fields[i].Bound != parse.BoundNone {
			boundField = &st.Fields[i]
		}
	}
	switch {
	case boundField != nil:
		doc += ", with " + boundField.Name + " set to the length of the data"
	case st.BoundPath != "":
		doc += ", with " + st.BoundPath + " set to the length of the data"
	}
	if st.Tokens {
		doc += ": each field that is set, as its token and value, in the order of the fields"
	}
	doc += ". It returns dst unchanged and a *dmrgo.FieldRangeError or another error when a field does not fit. It allocates only to grow dst."
	comment(f, doc+docCite(pdu))
	var err error
	f.Func().Params(Id("s").Op("*").Id(pdu.Name)).Id("AppendBinary").Params(Id("dst").Index().Byte()).Params(Index().Byte(), Error()).BlockFunc(func(g *Group) {
		if encMayFail(st) {
			g.Id("_n").Op(":=").Len(Id("dst"))
		}
		if st.Tokens {
			for _, fl := range st.Fields {
				if fl.Skip {
					continue
				}
				present := Id("s").Dot(fl.Name)
				if fl.Has != "" {
					present = Id("s").Dot(fl.Has)
				}
				g.If(present).BlockFunc(func(b *Group) {
					b.Id("dst").Op("=").Append(Id("dst"), hexByte(fl.Token))
					if er := e.encodeValue(b, fl, Id("s").Dot(fl.Name)); er != nil {
						err = er
					}
				})
			}
			g.Return(Id("dst"), Nil())
			return
		}
		var patch func(g *Group)
		for _, fl := range st.Fields {
			if fl.Skip {
				continue
			}
			one := func(b *Group) {
				if fl.Bound != parse.BoundNone {
					if fl.SKind == parse.StreamUintvar {
						b.Id("_lp").Op(":=").Len(Id("dst"))
						patch = func(g *Group) {
							g.Id("_l").Op(":=").Uint64().Call(Len(Id("dst")).Op("-").Id("_lp"))
							g.Id("_k").Op(":=").Qual(wirePkg, "UintvarLen").Call(Id("_l"))
							g.Id("_m").Op(":=").Len(Id("dst"))
							g.For(Range().Id("_k")).Block(Id("dst").Op("=").Append(Id("dst"), Lit(0)))
							g.Copy(Id("dst").Index(Id("_lp").Op("+").Id("_k"), Empty()), Id("dst").Index(Id("_lp"), Id("_m")))
							g.Qual(wirePkg, "PutUintvar").Call(Id("dst").Index(Id("_lp"), Id("_lp").Op("+").Id("_k")), Id("_l"))
						}
						return
					}
					b.Id("_lp").Op(":=").Len(Id("dst"))
					appendInt(b, fl.Octets, fl.LE, Lit(0))
					start := Parens(Id("_lp").Op("+").Lit(fl.Octets))
					if fl.Bound == parse.BoundTotal {
						start = Id("_n")
					}
					patch = func(g *Group) {
						w := 8 * fl.Octets
						g.Id("_l").Op(":=").Uint64().Call(Len(Id("dst")).Op("-").Add(start))
						g.If(Id("_l").Op(">>").Lit(w).Op("!=").Lit(0)).Block(e.rangeErr(fl, w, false, nil, nil))
						for i := range fl.Octets {
							shift := 8 * (fl.Octets - 1 - i)
							if fl.LE {
								shift = 8 * i
							}
							x := Id("_l")
							if shift > 0 {
								x = Id("_l").Op(">>").Lit(shift)
							}
							g.Id("dst").Index(Id("_lp").Op("+").Lit(i)).Op("=").Byte().Call(x)
						}
					}
					return
				}
				if st.BoundPath != "" {
					if head, _, _ := strings.Cut(st.BoundPath, "."); head == fl.Name {
						frame, sub, er := e.boundPatch(fl.Name)
						if er != nil {
							err = er
							return
						}
						b.Id("_fo").Op(":=").Len(Id("dst"))
						at := sub.BitStart / 8
						n := sub.BitWidth / 8
						fsize := frame.Frame.Sizes[0]
						start := Parens(Id("_fo").Op("+").Lit(fsize))
						if st.BoundMode == parse.BoundTotal {
							start = Id("_n")
						}
						patch = func(g *Group) {
							w := 8 * n
							g.Id("_l").Op(":=").Uint64().Call(Len(Id("dst")).Op("-").Add(start))
							bf := fl
							bf.Name = st.BoundPath
							g.If(Id("_l").Op(">>").Lit(w).Op("!=").Lit(0)).Block(e.rangeErr(bf, w, false, nil, nil))
							for i := range n {
								x := Id("_l")
								if sh := 8 * (n - 1 - i); sh > 0 {
									x = Id("_l").Op(">>").Lit(sh)
								}
								g.Id("dst").Index(Id("_fo").Op("+").Lit(at + i)).Op("=").Byte().Call(x)
							}
						}
					}
				}
				if er := e.encodeValue(b, fl, Id("s").Dot(fl.Name)); er != nil {
					err = er
				}
			}
			if fl.If != nil {
				g.If(e.cond("s", fl.If)).BlockFunc(one)
				continue
			}
			one(g)
		}
		if patch != nil {
			patch(g)
		}
		g.Return(Id("dst"), Nil())
	})
	return err
}

func intMayFail(f parse.StreamField) bool {
	w := 8 * f.Octets
	bw := basicWidth(f)
	if f.SignedInt {
		return bw == 0 || bw > w
	}
	return w < 64 && (bw == 0 || bw > w)
}

func encMayFail(st *parse.StreamDirective) bool {
	if st.BoundPath != "" {
		return true
	}
	for _, f := range st.Fields {
		if f.Skip {
			continue
		}
		switch {
		case f.Bound != parse.BoundNone:
			return true
		case f.SKind == parse.StreamInt && intMayFail(f):
			return true
		case f.SKind == parse.StreamFixed, f.SKind == parse.StreamUfloatvar, f.SKind == parse.StreamSfloatvar,
			f.SKind == parse.StreamFrame, f.SKind == parse.StreamNested, f.SKind == parse.StreamTokens:
			return true
		case f.SKind == parse.StreamBytes && (f.Length == parse.LengthExpr || f.Length == parse.LengthPrefix && !f.VarLen):
			return true
		}
	}
	return false
}

func emitStreamText(f *File, pdu parse.PDUStruct) {
	for _, fl := range pdu.Stream.Fields {
		if fl.Skip || fl.Text == parse.TextNone {
			continue
		}
		be := fl.Text == parse.TextUTF16BE
		order := "UTF-16LE"
		if be {
			order = "UTF-16BE"
		}
		comment(f, fl.Name+"UTF8 appends "+fl.Name+", "+order+" text, to dst as UTF-8 and returns dst. The text ends at the first NUL character and drops its trailing DEL characters. It allocates only to grow dst.")
		f.Func().Params(Id("s").Op("*").Id(pdu.Name)).Id(fl.Name + "UTF8").Params(Id("dst").Index().Byte()).Index().Byte().Block(
			Return(Qual(wirePkg, "AppendUTF8").Call(Id("dst"), Id("s").Dot(fl.Name), Lit(be))),
		)
		f.Line()
		nul, nulDoc := Null(), ""
		if fl.Nul {
			nul, nulDoc = Id("dst").Op("=").Append(Id("dst"), Lit(0), Lit(0)), ", then a NUL character"
		}
		comment(f, "Set"+fl.Name+"UTF8 appends text, UTF-8, to dst as "+order+nulDoc+", points "+fl.Name+" at the appended octets and returns dst. It allocates only to grow dst.")
		f.Func().Params(Id("s").Op("*").Id(pdu.Name)).Id("Set"+fl.Name+"UTF8").Params(Id("dst"), Id("text").Index().Byte()).Index().Byte().Block(
			Id("n").Op(":=").Len(Id("dst")),
			Id("dst").Op("=").Qual(wirePkg, "AppendUTF16").Call(Id("dst"), Id("text"), Lit(be)),
			nul,
			Id("s").Dot(fl.Name).Op("=").Id("dst").Index(Id("n"), Empty()),
			Return(Id("dst")),
		)
		f.Line()
	}
}

func emitStream(f *File, pdu parse.PDUStruct, types map[string]parse.PDUStruct, ev *errVars) error {
	if err := emitStreamDecode(f, pdu, types, ev); err != nil {
		return err
	}
	f.Line()
	if err := emitStreamEncode(f, pdu, types, ev); err != nil {
		return err
	}
	f.Line()
	emitStreamText(f, pdu)
	emitString(f, pdu, false)
	f.Line()
	return nil
}
