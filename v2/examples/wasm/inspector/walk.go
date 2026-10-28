package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

type bitTag struct {
	ranges   [][2]int
	opts     map[string]string
	stream   string
	hasBits  bool
	skip     bool
	fallback bool
}

func parseTag(tag string) bitTag {
	t := bitTag{opts: map[string]string{}}
	if tag == "-" {
		t.skip = true
		return t
	}
	for i, part := range strings.Split(tag, ",") {
		k, v, _ := strings.Cut(part, ":")
		switch {
		case k == "bits" || k == "bit":
			for _, r := range strings.Split(v, "+") {
				a, b, ok := strings.Cut(r, "-")
				lo, err1 := strconv.Atoi(a)
				hi := lo
				var err2 error
				if ok {
					hi, err2 = strconv.Atoi(b)
				}
				if err1 == nil && err2 == nil {
					t.ranges = append(t.ranges, [2]int{lo, hi + 1})
				}
			}
			t.hasBits = len(t.ranges) > 0
		case i == 0:
			t.stream = part
		default:
			t.opts[k] = v
		}
	}
	_, t.fallback = t.opts["fallback"]
	return t
}

func structBits(t reflect.Type) int {
	n := 0
	for i := 0; i < t.NumField(); i++ {
		bt := parseTag(t.Field(i).Tag.Get("dmr"))
		for _, r := range bt.ranges {
			n = max(n, r[1])
		}
	}
	return n
}

func usesBits(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if parseTag(t.Field(i).Tag.Get("dmr")).hasBits {
			return true
		}
	}
	return false
}

func walkBits(v reflect.Value, buf, base int) []*Node {
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	dispatched := false
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.IsExported() && v.Field(i).Kind() == reflect.Pointer && !v.Field(i).IsNil() && strings.Contains(f.Tag.Get("dmr"), "dispatch:") {
			dispatched = true
		}
	}
	var out []*Node
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		fv := v.Field(i)
		bt := parseTag(f.Tag.Get("dmr"))
		if bt.fallback && dispatched {
			continue
		}
		if fv.Kind() == reflect.Pointer && fv.IsNil() {
			continue
		}
		if hasOpt(bt, "union") {
			if vs := unionNodes(v, buf, base, bt); len(vs) > 0 {
				out = append(out, vs...)
				continue
			}
		}
		n := &Node{Name: f.Name}
		for _, r := range bt.ranges {
			if buf >= 0 {
				n.Ranges = append(n.Ranges, Range{Buf: buf, Start: base + r[0], End: base + r[1]})
			}
		}
		fillValue(n, fv, buf, base, bt)
		out = append(out, n)
	}
	return out
}

func hasOpt(bt bitTag, k string) bool {
	_, ok := bt.opts[k]
	return ok
}

func unionNodes(v reflect.Value, buf, base int, bt bitTag) []*Node {
	p := reflect.New(v.Type())
	p.Elem().Set(v)
	var out []*Node
	for i := 0; i < p.NumMethod(); i++ {
		m := p.Type().Method(i)
		mt := m.Type
		if !strings.HasPrefix(m.Name, "As") || mt.NumIn() != 1 || mt.NumOut() != 2 || mt.Out(1).Kind() != reflect.Bool || mt.Out(0).Kind() != reflect.Struct {
			continue
		}
		r := p.Method(i).Call(nil)
		if !r[1].Bool() {
			continue
		}
		n := &Node{Name: strings.TrimPrefix(m.Name, "As")}
		for _, rg := range bt.ranges {
			if buf >= 0 {
				n.Ranges = append(n.Ranges, Range{Buf: buf, Start: base + rg[0], End: base + rg[1]})
			}
		}
		fillValue(n, r[0], buf, base, bt)
		out = append(out, n)
	}
	return out
}

func fillValue(n *Node, fv reflect.Value, buf, base int, bt bitTag) {
	if fv.Type() == reflect.TypeFor[fec.Result]() {
		r, _ := fv.Interface().(fec.Result)
		n.Value = fecText(r)
		if r.Uncorrectable {
			n.Err = "FEC or CRC check failed"
		}
		return
	}
	ev := reflect.Indirect(fv)
	if ev.Kind() == reflect.Struct {
		n.Value = typeName(ev.Type())
		if s, ok := stringOf(fv); ok && len(s) < 60 && !strings.Contains(s, "{") {
			n.Value = s
		}
		sub := base
		if len(bt.ranges) > 0 {
			sub = base + bt.ranges[0][0]
		}
		if usesBits(ev.Type()) {
			n.Children = walkBits(ev, buf, sub)
		} else {
			n.Children = walkPlain(ev)
		}
		return
	}
	n.Value = scalarText(fv, bt)
}

func walkPlain(v reflect.Value) []*Node {
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct {
		return nil
	}
	var out []*Node
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		fv := v.Field(i)
		if fv.Kind() == reflect.Pointer && fv.IsNil() {
			continue
		}
		if bt := parseTag(f.Tag.Get("dmr")); hasOpt(bt, "union") {
			if vs := unionNodes(v, -1, 0, bt); len(vs) > 0 {
				out = append(out, vs...)
				continue
			}
		}
		n := &Node{Name: f.Name}
		fillValue(n, fv, -1, 0, bitTag{})
		out = append(out, n)
	}
	return out
}

func walkStream(v reflect.Value, data []byte, buf, off int) []*Node {
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	cur := 0
	var out []*Node
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		fv := v.Field(i)
		tag := f.Tag.Get("dmr")
		bt := parseTag(tag)
		_, cond := bt.opts["if"]
		if cond && fv.IsZero() {
			continue
		}
		n := &Node{Name: f.Name}
		start, size := -1, -1
		if p, ok := slicePos(fv, data); ok {
			start, size = p, fv.Len()
			if strings.HasPrefix(bt.stream, "lv") && cur >= 0 {
				start = cur
				size = p + fv.Len() - cur
			}
		} else if cur >= 0 {
			start = cur
			size = streamSize(bt, fv)
		}
		if start >= 0 && size > 0 && start+size <= len(data) {
			n.Ranges = []Range{{Buf: buf, Start: (off + start) * 8, End: (off + start + size) * 8}}
			cur = start + size
		} else if size < 0 {
			cur = -1
		}
		ev := reflect.Indirect(fv)
		switch {
		case ev.Kind() == reflect.Struct && usesBits(ev.Type()) && start >= 0:
			n.Value = typeName(ev.Type())
			if s, ok := stringOf(fv); ok && len(s) < 60 && !strings.Contains(s, "{") {
				n.Value = s
			}
			n.Children = walkBits(ev, buf, (off+start)*8)
		case ev.Kind() == reflect.Struct && start >= 0 && size > 0:
			n.Value = typeName(ev.Type())
			n.Children = walkStream(ev, data[start:start+size], buf, off+start)
		default:
			fillValue(n, fv, -1, 0, bt)
			if len(n.Ranges) == 0 {
				clearRanges(n.Children)
			}
		}
		out = append(out, n)
	}
	return out
}

func clearRanges(ns []*Node) {
	for _, n := range ns {
		n.Ranges = nil
		clearRanges(n.Children)
	}
}

func streamSize(bt bitTag, fv reflect.Value) int {
	s := bt.stream
	switch {
	case s == "u8" || s == "i8":
		return 1
	case s == "u16" || s == "i16":
		return 2
	case s == "u24" || s == "i24":
		return 3
	case s == "u32" || s == "i32":
		return 4
	case strings.HasPrefix(s, "bytes:"):
		n, _ := strconv.Atoi(strings.TrimPrefix(s, "bytes:"))
		return n
	case s == "frame":
		ev := reflect.Indirect(fv)
		if ev.Kind() == reflect.Struct && usesBits(ev.Type()) {
			return (structBits(ev.Type()) + 7) / 8
		}
	}
	return -1
}

func slicePos(fv reflect.Value, data []byte) (int, bool) {
	if fv.Kind() != reflect.Slice || fv.Type().Elem().Kind() != reflect.Uint8 || fv.Len() == 0 || len(data) == 0 {
		return 0, false
	}
	p := fv.Pointer()
	b := uintptr(unsafe.Pointer(unsafe.SliceData(data)))
	if p < b || p+uintptr(fv.Len()) > b+uintptr(len(data)) {
		return 0, false
	}
	return int(p - b), true
}

func typeName(t reflect.Type) string {
	return t.String()
}

func stringOf(v reflect.Value) (string, bool) {
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return "", false
	}
	if s, ok := v.Interface().(fmt.Stringer); ok {
		return s.String(), true
	}
	if v.Kind() == reflect.Pointer {
		return "", false
	}
	p := reflect.New(v.Type())
	p.Elem().Set(v)
	if s, ok := p.Interface().(fmt.Stringer); ok {
		return s.String(), true
	}
	return "", false
}

func scalarText(v reflect.Value, bt bitTag) string {
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		u := v.Uint()
		num := fmt.Sprintf("%d (0x%X)", u, u)
		if s, ok := stringOf(v); ok && v.Type().PkgPath() != "" && s != strconv.FormatUint(u, 10) {
			return s + " = " + num
		}
		return num
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		if s, ok := stringOf(v); ok && v.Type().PkgPath() != "" {
			return fmt.Sprintf("%s = %d", s, v.Int())
		}
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.String:
		return strconv.Quote(v.String())
	case reflect.Array, reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			b := make([]byte, v.Len())
			reflect.Copy(reflect.ValueOf(b), v)
			if s, ok := textOf(b, bt); ok {
				return s
			}
			if len(b) == 0 {
				return "(empty)"
			}
			return hex.EncodeToString(b)
		}
		parts := make([]string, 0, v.Len())
		for i := 0; i < v.Len() && i < 32; i++ {
			parts = append(parts, scalarText(v.Index(i), bitTag{}))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return "nil"
		}
		return scalarText(v.Elem(), bt)
	default:
	}
	if s, ok := stringOf(v); ok {
		return s
	}
	return fmt.Sprintf("%v", v.Interface())
}

func textOf(b []byte, bt bitTag) (string, bool) {
	_, isText := bt.opts["text"]
	if !isText {
		if len(b) < 3 || bytes.Count(b, []byte{0}) == len(b) {
			return "", false
		}
		for _, c := range b {
			if (c < 0x20 || c > 0x7E) && c != 0 && c != ' ' {
				return "", false
			}
		}
	}
	s := strings.TrimRight(string(b), " \x00")
	if !utf8.ValidString(s) {
		return "", false
	}
	return fmt.Sprintf("%q (%s)", s, hex.EncodeToString(b)), true
}

func fecText(r fec.Result) string {
	s := fmt.Sprintf("%d of %d bits corrected", r.ErrorsCorrected, r.BitsChecked)
	if r.BitsChecked == 0 {
		s = "check passed"
		if r.Uncorrectable {
			s = "check failed"
		}
		return s
	}
	if r.Uncorrectable {
		s += ", uncorrectable"
	}
	return s
}
