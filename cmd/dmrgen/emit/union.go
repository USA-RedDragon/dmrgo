package emit

import (
	"strconv"
	"strings"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
	. "github.com/dave/jennifer/jen"
)

func emitUnion(f *File, pdu parse.PDUStruct, ev *errVars) error {
	union := pdu.UnionField()
	for _, g := range pdu.Getters {
		if err := emitGetter(f, pdu, union, g); err != nil {
			return err
		}
	}
	for _, v := range pdu.Variants {
		if err := emitAs(f, pdu, union, v); err != nil {
			return err
		}
		if err := emitSet(f, pdu, union, v, ev); err != nil {
			return err
		}
	}
	return nil
}

func emitGetter(f *File, pdu parse.PDUStruct, union *parse.Field, g parse.Field) error {
	rel := g
	rel.BitStart -= union.BitStart
	rel.BitEnd -= union.BitStart
	rel.ExtraBitRanges = nil
	for _, r := range g.ExtraBitRanges {
		rel.ExtraBitRanges = append(rel.ExtraBitRanges, [2]int{r[0] - union.BitStart, r[1] - union.BitStart})
	}
	buf := func() *Statement { return Id("s").Dot(union.Name) }
	zero := Lit(0)
	if g.Kind == parse.FieldBool {
		zero = False()
	}
	text := g.Name + " returns " + g.Doc + "."
	if g.OnlyField != "" {
		text += " It returns " + map[bool]string{true: "false", false: "0"}[g.Kind == parse.FieldBool] + " unless " + condText(parse.VariantCond{Field: g.OnlyField, Values: g.OnlyValues}) + "."
	}
	comment(f, text)
	var genErr error
	f.Func().Params(Id("s").Op("*").Id(pdu.Name)).Id(g.Name).Params().Add(qualType(g, pdu)).BlockFunc(func(b *Group) {
		if g.OnlyField != "" {
			cond, err := condCode(pdu, parse.VariantCond{Field: g.OnlyField, Values: g.OnlyValues})
			if err != nil {
				genErr = err
				return
			}
			b.If(Op("!").Parens(cond)).Block(Return(zero))
		}
		if g.Kind == parse.FieldBool {
			b.Return(buf().Index(Lit(rel.BitStart / 8)).Op("&").Add(hexByte(0x80 >> (rel.BitStart % 8))).Op("!=").Lit(0))
			return
		}
		ranges := rel.Ranges()
		if len(ranges) == 1 {
			b.Return(castUint(g, pdu, readPacked(buf, rel.BitStart, rel.BitEnd-rel.BitStart+1)))
			return
		}
		for i, r := range ranges {
			read := readPacked(buf, r[0], r[1]-r[0]+1)
			if i == 0 {
				b.Id("v").Op(":=").Add(read)
				continue
			}
			b.Id("v").Op("<<=").Lit(r[1] - r[0] + 1)
			b.Id("v").Op("|=").Add(read)
		}
		b.Return(castUint(g, pdu, Id("v")))
	})
	f.Line()
	return genErr
}

func condField(pdu parse.PDUStruct, c parse.VariantCond) *Statement {
	if pdu.GetterByName(c.Field) != nil {
		return Id("s").Dot(c.Field).Call()
	}
	return Id("s").Dot(c.Field)
}

func condCode(pdu parse.PDUStruct, c parse.VariantCond) (*Statement, error) {
	if c.Guard != nil {
		return guardCondition(condField(pdu, c), c.Guard)
	}
	e := condField(pdu, c).Op("==").Add(emitDispatchConstant(c.Values[0], pdu))
	for _, v := range c.Values[1:] {
		e = e.Op("||").Add(condField(pdu, c)).Op("==").Add(emitDispatchConstant(v, pdu))
	}
	return e, nil
}

func condsCode(pdu parse.PDUStruct, conds []parse.VariantCond) (*Statement, error) {
	var e *Statement
	for _, c := range conds {
		x, err := condCode(pdu, c)
		if err != nil {
			return nil, err
		}
		if len(conds) > 1 && len(c.Values) > 1 {
			x = Parens(x)
		}
		if e == nil {
			e = x
			continue
		}
		e = e.Op("&&").Add(x)
	}
	return e, nil
}

func condText(c parse.VariantCond) string {
	if c.Guard != nil {
		if c.Guard.Op == "!=" {
			return c.Field + " is not " + c.Guard.Value
		}
		return c.Field + " is " + c.Guard.Value
	}
	return c.Field + " is " + orList(c.Values)
}

func orList(vals []string) string {
	if len(vals) == 1 {
		return vals[0]
	}
	return strings.Join(vals[:len(vals)-1], ", ") + " or " + vals[len(vals)-1]
}

func andList(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func emitAs(f *File, pdu parse.PDUStruct, union *parse.Field, v parse.Variant) error {
	texts := make([]string, len(v.Conds))
	for i, c := range v.Conds {
		texts[i] = condText(c)
	}
	comment(f, "As"+v.Name+" decodes "+union.Name+" as "+article(v.Type)+" "+v.Type+" payload. ok is false unless "+andList(texts)+".")
	cond, err := condsCode(pdu, v.Conds)
	if err != nil {
		return err
	}
	f.Func().Params(Id("s").Op("*").Id(pdu.Name)).Id("As"+v.Name).Params().Params(Id("p").Id(v.Type), Id("ok").Bool()).Block(
		If(Op("!").Parens(cond)).Block(Return(Id("p"), False())),
		Return(Id(pdu.DecodeFuncName(v.Type)).Call(Id("s").Dot(union.Name)), True()),
	)
	f.Line()
	return nil
}

type setCond struct {
	cond  parse.VariantCond
	param string
	value Code
	text  string
}

func settable(pdu parse.PDUStruct, c parse.VariantCond) bool {
	if pdu.GetterByName(c.Field) != nil || pdu.IsInput(c.Field) {
		return false
	}
	return c.Guard == nil || c.Guard.Op == "=="
}

func setParamName(field string) string {
	name := inputParamName(field)
	if strings.ToUpper(field) == field {
		name = strings.ToLower(field)
	}
	switch name {
	case "s", "p", "raw", "err", "t", "ok":
		return "_" + name
	}
	return name
}

type preCheck struct {
	field    parse.Field
	min, max uint64
	text     string
}

func shifted(r [][2]int, by int) [][2]int {
	out := make([][2]int, len(r))
	for i, x := range r {
		out[i] = [2]int{x[0] - by, x[1] - by}
	}
	return out
}

func sameRanges(a, b [][2]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func fixedByConst(t *parse.PDUStruct, ranges [][2]int, c parse.VariantCond) bool {
	for _, k := range t.Consts {
		if !sameRanges(k.Ranges, ranges) {
			continue
		}
		if c.Guard != nil {
			return c.Guard.Op == "==" && k.Value == c.Guard.Value
		}
		return len(c.Values) == 1 && c.Values[0] == k.Value
	}
	return false
}

func payloadCheck(pdu parse.PDUStruct, union *parse.Field, v parse.Variant, c parse.VariantCond) (fixed bool, pc *preCheck) {
	g := pdu.GetterByName(c.Field)
	t := pdu.Types[v.Type]
	if g == nil || t == nil {
		return false, nil
	}
	ranges := shifted(g.Ranges(), union.BitStart)
	if fixedByConst(t, ranges, c) {
		return true, nil
	}
	f := t.FieldByName(c.Field)
	if f == nil || !sameRanges(f.Ranges(), ranges) || c.Guard == nil || c.Guard.Op != "!=" || f.Kind == parse.FieldBool {
		return false, nil
	}
	n, err := strconv.ParseUint(c.Guard.Value, 10, 64)
	top := uint64(1)<<f.BitWidth - 1
	switch {
	case err != nil:
		return false, nil
	case n == 0:
		return false, &preCheck{field: *f, min: 1, max: top, text: "p." + f.Name + " is 0"}
	case n == top:
		return false, &preCheck{field: *f, min: 0, max: top - 1, text: "p." + f.Name + " is " + c.Guard.Value}
	}
	return false, nil
}

func emitSet(f *File, pdu parse.PDUStruct, union *parse.Field, v parse.Variant, ev *errVars) error {
	var sets []setCond
	var pres []preCheck
	check := false
	var params []Code
	for _, c := range v.Conds {
		if !settable(pdu, c) {
			fixed, pc := payloadCheck(pdu, union, v, c)
			switch {
			case pc != nil:
				pres = append(pres, *pc)
			case !fixed:
				check = true
			}
			continue
		}
		switch {
		case c.Guard != nil:
			sets = append(sets, setCond{cond: c, value: Id(c.Guard.Value), text: c.Field + " to " + c.Guard.Value})
		case len(c.Values) == 1:
			sets = append(sets, setCond{cond: c, value: emitDispatchConstant(c.Values[0], pdu), text: c.Field + " to " + c.Values[0]})
		default:
			name := setParamName(c.Field)
			sets = append(sets, setCond{cond: c, param: name, value: Id(name), text: c.Field + " to " + name})
			params = append(params, Id(name).Add(qualType(*pdu.FieldByName(c.Field), pdu)))
		}
	}
	params = append(params, Id("p").Id(v.Type))

	texts := make([]string, 0, len(sets)+1)
	for _, sc := range sets {
		texts = append(texts, sc.text)
	}
	texts = append(texts, union.Name)
	doc := "Set" + v.Name + " makes s carry p as " + article(v.Type) + " " + v.Type + " payload: it sets " + andList(texts) + ", and keeps the other fields."
	var errs []string
	for _, sc := range sets {
		if sc.param != "" {
			errs = append(errs, "ErrWrongOpcode when "+sc.param+" is not "+orList(sc.cond.Values))
		}
	}
	rangeErr := "a *dmrgo.FieldRangeError when a field of p does not fit its bits"
	for _, pc := range pres {
		rangeErr += " or " + pc.text
	}
	errs = append(errs, rangeErr)
	if check {
		errs = append(errs, "an error when the payload does not meet the conditions of As"+v.Name)
	}
	doc += " It returns " + andList(errs) + ", and then leaves s as it is."
	comment(f, doc)

	var genErr error
	f.Func().Params(Id("s").Op("*").Id(pdu.Name)).Id("Set" + v.Name).Params(params...).Error().BlockFunc(func(b *Group) {
		for _, sc := range sets {
			if sc.param == "" {
				continue
			}
			cond := Id(sc.param).Op("==").Add(emitDispatchConstant(sc.cond.Values[0], pdu))
			for _, val := range sc.cond.Values[1:] {
				cond = cond.Op("||").Id(sc.param).Op("==").Add(emitDispatchConstant(val, pdu))
			}
			b.If(Op("!").Parens(cond)).Block(Return(Id("ErrWrongOpcode")))
		}
		for _, pc := range pres {
			def := Op("&").Qual(rootPkg, "FieldRangeError").Values(Dict{
				Id("Type"):  Lit(v.Type),
				Id("Field"): Lit(pc.field.Name),
				Id("Bits"):  Lit(pc.field.BitWidth),
				Id("Min"):   Lit(float64(pc.min)),
				Id("Max"):   Lit(float64(pc.max)),
			})
			name := "err" + pdu.Name + "Set" + v.Name + pc.field.Name
			bad := Id("p").Dot(pc.field.Name).Op("<").Id(strconv.FormatUint(pc.min, 10)).Op("||").Id("p").Dot(pc.field.Name).Op(">").Id(strconv.FormatUint(pc.max, 10))
			if pc.min == 0 {
				bad = Id("p").Dot(pc.field.Name).Op(">").Id(strconv.FormatUint(pc.max, 10))
			}
			b.If(bad).Block(Return(ev.get(name, def)))
		}
		b.List(Id("raw"), Err()).Op(":=").Id("p").Dot(pdu.EncodeFuncName(v.Type)).Call()
		b.If(Err().Op("!=").Nil()).Block(Return(Err()))
		dst := "s"
		if check {
			dst = "t"
			b.Id("t").Op(":=").Op("*").Id("s")
		}
		for _, sc := range sets {
			b.Id(dst).Dot(sc.cond.Field).Op("=").Add(sc.value)
		}
		b.Id(dst).Dot(union.Name).Op("=").Id("raw")
		if check {
			name := "err" + pdu.Name + v.Name + "Payload"
			b.If(List(Id("_"), Id("ok")).Op(":=").Id("t").Dot("As"+v.Name).Call(), Op("!").Id("ok")).Block(
				Return(ev.get(name, errorsNew(pdu.Name+".Set"+v.Name+": the payload does not meet the conditions of As"+v.Name))),
			)
			b.Op("*").Id("s").Op("=").Id("t")
		}
		b.Return(Nil())
	})
	f.Line()
	return genErr
}
