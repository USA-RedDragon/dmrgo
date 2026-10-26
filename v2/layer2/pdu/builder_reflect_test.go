//go:build !tinygo

package pdu_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

var unionTypes = []reflect.Type{
	reflect.TypeFor[pdu.CSBK](),
	reflect.TypeFor[pdu.MBCHeader](),
	reflect.TypeFor[pdu.FullLinkControl](),
	reflect.TypeFor[pdu.ShortLC](),
	reflect.TypeFor[pdu.DataHeader](),
}

var errorType = reflect.TypeFor[error]()

func builderArgs(m reflect.Type, skip int) []reflect.Value {
	args := make([]reflect.Value, 0, m.NumIn())
	for i := skip; i < m.NumIn(); i++ {
		if m.In(i) == reflect.TypeFor[enums.FLCO]() {
			args = append(args, reflect.ValueOf(enums.FLCOTalkerAliasBlock2))
			continue
		}
		args = append(args, reflect.Zero(m.In(i)))
	}
	return args
}

func TestBuildersMatchSetters(t *testing.T) {
	t.Parallel()

	n := 0
	for _, ut := range unionTypes {
		pt := reflect.PointerTo(ut)
		for i := range pt.NumMethod() {
			set := pt.Method(i)
			if !strings.HasPrefix(set.Name, "Set") || set.Type.NumIn() < 2 || set.Type.NumOut() != 1 || set.Type.Out(0) != errorType {
				continue
			}
			payload := set.Type.In(set.Type.NumIn() - 1)
			if payload.Kind() != reflect.Struct {
				continue
			}
			build, ok := payload.MethodByName(ut.Name())
			if !ok {
				t.Errorf("%s has %s but %s has no method %s", ut.Name(), set.Name, payload.Name(), ut.Name())
				continue
			}
			if build.Type.NumOut() != 2 || build.Type.Out(0) != ut || build.Type.Out(1) != errorType {
				t.Errorf("%s.%s returns %v", payload.Name(), ut.Name(), build.Type)
				continue
			}
			n++
			params := builderArgs(set.Type, 1)
			want := reflect.New(ut)
			if ut == reflect.TypeFor[pdu.CSBK]() {
				want.Elem().FieldByName("LastBlock").SetBool(true)
			}
			setErr := set.Func.Call(append([]reflect.Value{want}, params...))[0]
			out := build.Func.Call(append([]reflect.Value{params[len(params)-1]}, params[:len(params)-1]...))
			if setErr.IsNil() != out[1].IsNil() {
				t.Errorf("%s.%s: error %v, %s error %v", payload.Name(), ut.Name(), out[1], set.Name, setErr)
				continue
			}
			if !setErr.IsNil() {
				want = reflect.New(ut)
			}
			if !reflect.DeepEqual(out[0].Interface(), want.Elem().Interface()) {
				t.Errorf("%s.%s = %+v, want %+v", payload.Name(), ut.Name(), out[0], want.Elem())
			}
		}
	}
	if n < 60 {
		t.Fatalf("checked %d builders, want every SetX payload of the union PDUs", n)
	}
}

func TestBuildersRebuildCaptures(t *testing.T) {
	t.Parallel()

	counts := map[string]int{}
	for _, b := range captureBursts(t) {
		if p, _ := b.Sync(); !isDataSync(p) {
			continue
		}
		payload, res := b.BPTCPayload()
		if res.Uncorrectable {
			continue
		}
		var v reflect.Value
		switch dt := b.SlotType().DataType; dt {
		case enums.DataTypeCSBK:
			c := pdu.DecodeCSBK(payload)
			v = reflect.ValueOf(&c)
		case enums.DataTypeDataHeader:
			h := pdu.DecodeDataHeader(payload)
			v = reflect.ValueOf(&h)
		case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
			lc := pdu.DecodeFullLinkControl(payload, dt)
			v = reflect.ValueOf(&lc)
		default:
			continue
		}
		if fecOf(v.Interface()).Uncorrectable {
			continue
		}
		if name, ok := rebuild(t, v, payload); ok {
			counts[name]++
		}
	}
	for _, kind := range []string{"CSBK", "DataHeader", "FullLinkControl"} {
		if counts[kind] == 0 {
			t.Errorf("no captured %s rebuilt: %v", kind, counts)
		}
	}
}

func rebuild(t *testing.T, v reflect.Value, want [12]byte) (string, bool) {
	t.Helper()
	ut := v.Elem().Type()
	for i := range v.Type().NumMethod() {
		as := v.Type().Method(i)
		if !strings.HasPrefix(as.Name, "As") || as.Type.NumIn() != 1 || as.Type.NumOut() != 2 {
			continue
		}
		out := as.Func.Call([]reflect.Value{v})
		if !out[1].Bool() {
			continue
		}
		build, ok := out[0].Type().MethodByName(ut.Name())
		if !ok || build.Type.NumIn() != 1 {
			continue
		}
		res := build.Func.Call([]reflect.Value{out[0]})
		if !res[1].IsNil() {
			t.Errorf("%s from %s: %v", ut.Name(), as.Name, res[1])
			return "", false
		}
		got := reflect.New(ut)
		got.Elem().Set(res[0])
		for _, f := range []string{"ProtectFlag", "LastBlock", "Reserved", "BurstDataType", "Hint0x38"} {
			if g := got.Elem().FieldByName(f); g.IsValid() {
				g.Set(v.Elem().FieldByName(f))
			}
		}
		enc := got.MethodByName("Encode").Call(nil)
		if !enc[1].IsNil() || !reflect.DeepEqual(enc[0].Interface(), want) {
			t.Errorf("%s from %s: %x %v, want %x", ut.Name(), as.Name, enc[0], enc[1], want)
		}
		return ut.Name(), true
	}
	return "", false
}
