//go:build !tinygo

// TinyGo has no reflect.Type.NumIn or reflect.Value.Call, which this test needs.

package layer2_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// bitFuncs holds every exported function and method of the v2 module that
// takes bit.Bit values, except those of package bit (see bit_test.go) and of
// the cgo codec2 binding. Methods are wrapped with a fresh receiver.

var bitFuncs = map[string]any{
	"layer2/framer.Search": func(stream [600]bit.Bit) (framer.Match, bool) {
		return framer.Search(stream[:], 0, -1)
	},
	"layer2/framer.Framer.Write": func(stream [1200]bit.Bit) []framer.Burst {
		return framer.New(framer.Config{}).Write(stream[:]...)
	},
	"layer2/framer.Framer.AppendBursts": func(stream [1200]bit.Bit) []framer.Burst {
		return framer.New(framer.Config{}).AppendBursts(nil, stream[:]...)
	},
	"layer2.VoiceTransmitter.AddSlots": func(slots ...[72]bit.Bit) ([]layer2.RawBurst, error) {
		return voiceTransmitter().AddSlots(slots...)
	},
	"layer2.VoiceTransmitter.AddBurst": func(slots [layer2.VoiceSlotsPerBurst][72]bit.Bit) (layer2.RawBurst, error) {
		return voiceTransmitter().AddBurst(&slots)
	},
	"layer2.VoiceTransmitter.AppendSlots": func(slots ...[72]bit.Bit) ([]layer2.RawBurst, error) {
		return voiceTransmitter().AppendSlots(nil, slots...)
	},
	"layer2.VoiceTransmitter.AppendFrames": func(frames ...vocoder.AMBEFrame) ([]layer2.RawBurst, error) {
		return voiceTransmitter().AppendFrames(nil, frames...)
	},
	"voice.Transmitter.Slot": func(s [72]bit.Bit) []layer2.RawBurst {
		return transmitVoice(func(tx *voice.Transmitter) error { return tx.Slot(s) })
	},
	"voice.Transmitter.Slots": func(s ...[72]bit.Bit) []layer2.RawBurst {
		return transmitVoice(func(tx *voice.Transmitter) error { return tx.Slots(s...) })
	},
	"voice.Transmitter.Frame": func(f vocoder.AMBEFrame) []layer2.RawBurst {
		return transmitVoice(func(tx *voice.Transmitter) error { return tx.Frame(f) })
	},
	"voice.Transmitter.Frames": func(f ...vocoder.AMBEFrame) []layer2.RawBurst {
		return transmitVoice(func(tx *voice.Transmitter) error { return tx.Frames(f...) })
	},
	"layer2.Burst.SetAMBEFrames": func(frames [3]vocoder.AMBEFrame) [3][72]bit.Bit {
		var b layer2.Burst
		b.SetAMBEFrames(frames)
		return b.VoiceSlots
	},
	"layer2.RawBurst.SetAMBEFrames": func(frames [3]vocoder.AMBEFrame) layer2.RawBurst {
		var r layer2.RawBurst
		r.SetAMBEFrames(frames)
		return r
	},
	"layer2.BuildVoiceBurst": func(frames [3]vocoder.AMBEFrame) (layer2.RawBurst, error) {
		return layer2.BuildVoiceBurst(frames, enums.VoiceBurstB, pdu.EmbeddedSignalling{ColorCode: 1}, [4]byte{1, 2, 3, 4}, layer2.SyncSourceBS)
	},
	"vocoder.PackAMBEVoice": vocoder.PackAMBEVoice,
	"vocoder.PackAMBEVoiceFlags": func(frames [3]vocoder.AMBEFrame) [19]byte {
		return vocoder.PackAMBEVoiceFlags(frames, [3]bool{true, true, true})
	},
	"layer2.VoiceTransmitter.AddFrames": func(frames ...vocoder.AMBEFrame) ([]layer2.RawBurst, error) {
		return voiceTransmitter().AddFrames(frames...)
	},
	"internal/bits.Pack": func(src [96]bit.Bit) [12]byte {
		var dst [12]byte
		bits.Pack(dst[:], 0, src[:])
		return dst
	},
	"internal/bits.Unpack": func(dst [16]bit.Bit) [16]bit.Bit {
		bits.Unpack(dst[:], []byte{0xA5, 0x3C}, 0)
		return dst
	},
	"vocoder.DecodeAMBE72": vocoder.DecodeAMBE72,
	"vocoder.EncodeAMBE72": vocoder.EncodeAMBE72,
}

// transmitVoice sends a call whose voice add queues with a voice.Transmitter
// and returns its bursts.
func transmitVoice(add func(tx *voice.Transmitter) error) []layer2.RawBurst {
	tx := must(voice.NewTransmitter(&voice.TransmitterConfig{Queue: voice.MaxQueue}))
	if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, nil); err != nil {
		panic(err)
	}
	if err := add(tx); err != nil {
		panic(err)
	}
	tx.Stop()
	var out []layer2.RawBurst
	for {
		at, ok := tx.Deadline()
		if !ok {
			return out
		}
		for _, o := range tx.Tick(at, nil) {
			if o.Kind == voice.OutputBurst {
				out = append(out, o.Burst)
			}
		}
	}
}

func voiceTransmitter() *layer2.VoiceTransmitter {
	return must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1, Source: layer2.SyncSourceMS, Silence: [72]bit.Bit{}}))
}

// bitFuncNames lists the exported functions and methods of the v2 module
// whose parameters contain bit.Bit, keyed as in bitFuncs.
func bitFuncNames(t *testing.T) []string {
	t.Helper()
	root := ".."
	var names []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == "bit" || rel == filepath.Join("internal", "lowbit") || rel == filepath.Join("internal", "fecref") || rel == filepath.Join("vocoder", "codec2") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() || !takesBits(fd.Type.Params) {
				continue
			}
			name := pkg + "." + fd.Name.Name
			if fd.Recv != nil {
				name = pkg + "." + recvName(fd.Recv.List[0].Type) + "." + fd.Name.Name
			}
			names = append(names, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func takesBits(params *ast.FieldList) bool {
	found := false
	for _, p := range params.List {
		ast.Inspect(p.Type, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if x, ok := n.X.(*ast.Ident); ok && (x.Name == "bit" && n.Sel.Name == "Bit" || x.Name == "vocoder" && n.Sel.Name == "AMBEFrame") {
					found = true
				}
			case *ast.Ident:
				found = found || n.Name == "AMBEFrame"
			}
			return !found
		})
	}
	return found
}

func recvName(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// fillArg returns a value of type typ whose bit.Bit elements come from next.
// Other values are zero, and function values return zero values.
func fillArg(typ reflect.Type, next func() bit.Bit) reflect.Value {
	bitType := reflect.TypeFor[bit.Bit]()
	v := reflect.New(typ).Elem()
	switch {
	case typ == bitType:
		v.Set(reflect.ValueOf(next()))
	case typ.Kind() == reflect.Array:
		for i := range v.Len() {
			v.Index(i).Set(fillArg(typ.Elem(), next))
		}
	case typ.Kind() == reflect.Slice:
		n := 96
		if typ.Elem() != bitType {
			n = 3
		}
		v.Set(reflect.MakeSlice(typ, n, n))
		for i := range n {
			v.Index(i).Set(fillArg(typ.Elem(), next))
		}
	case typ.Kind() == reflect.Func:
		v.Set(reflect.MakeFunc(typ, func([]reflect.Value) []reflect.Value {
			out := make([]reflect.Value, typ.NumOut())
			for i := range out {
				out[i] = reflect.Zero(typ.Out(i))
			}
			return out
		}))
	}
	return v
}

// callWithBits calls fn with arguments whose bit.Bit elements come from next
// and returns its results.
func callWithBits(t *testing.T, name string, fn any, next func() bit.Bit) (out []any) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s panicked: %v", name, r)
			out = nil
		}
	}()
	fv := reflect.ValueOf(fn)
	ft := fv.Type()
	args := make([]reflect.Value, ft.NumIn())
	for i := range args {
		args[i] = fillArg(ft.In(i), next)
	}
	var res []reflect.Value
	if ft.IsVariadic() {
		res = fv.CallSlice(args)
	} else {
		res = fv.Call(args)
	}
	for _, r := range res {
		out = append(out, r.Interface())
	}
	return out
}

// TestBitValuesOtherThanZeroOrOne checks that every function that takes
// bit.Bit values reads only the low bit of each value: it must not panic, and
// it must return the same as for the values masked to their low bit. A
// soft-decision demodulator or a caller bug can pass such values.
func TestBitValuesOtherThanZeroOrOne(t *testing.T) {
	t.Parallel()

	names := bitFuncNames(t)
	if len(names) == 0 {
		t.Fatal("found no functions that take bit.Bit")
	}
	for _, name := range names {
		if _, ok := bitFuncs[name]; !ok {
			t.Errorf("bitFuncs has no entry for %s", name)
		}
	}
	listed := map[string]bool{}
	for _, name := range names {
		listed[name] = true
	}
	for name := range bitFuncs {
		if !listed[name] {
			t.Errorf("bitFuncs entry %s is not an exported function that takes bit.Bit", name)
		}
	}

	type pattern struct {
		name string
		gen  func(seed uint64) func() bit.Bit
	}
	patterns := []pattern{}
	for _, v := range []bit.Bit{2, 3, 7, 0x30, 0xFE, 0xFF} {
		patterns = append(patterns, pattern{"constant", func(uint64) func() bit.Bit {
			return func() bit.Bit { return v }
		}})
	}
	for range 8 {
		patterns = append(patterns, pattern{"random", func(seed uint64) func() bit.Bit {
			rng := rand.New(rand.NewPCG(seed, 1))
			return func() bit.Bit { return bit.Bit(rng.UintN(256)) }
		}})
	}
	for name, fn := range bitFuncs {
		for i, p := range patterns {
			seed := uint64(i)
			raw := callWithBits(t, name, fn, p.gen(seed))
			maskedGen := p.gen(seed)
			masked := callWithBits(t, name, fn, func() bit.Bit { return maskedGen() & 1 })
			if raw != nil && masked != nil && !reflect.DeepEqual(raw, masked) {
				t.Errorf("%s: %s values give %v, their low bits give %v", name, p.name, raw, masked)
				break
			}
		}
	}
}
