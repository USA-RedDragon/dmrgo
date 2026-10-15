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
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/fec/golay"
	"github.com/USA-RedDragon/dmrgo/v2/fec/hamming"
	"github.com/USA-RedDragon/dmrgo/v2/fec/quadraticresidue"
	"github.com/USA-RedDragon/dmrgo/v2/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// bitFuncs holds every exported function and method of the v2 module that
// takes bit.Bit values, except those of package bit (see bit_test.go) and of
// the cgo codec2 binding. Methods are wrapped with a fresh receiver.
//
//nolint:gochecknoglobals
var bitFuncs = map[string]any{
	"fec/bptc.Decode196": bptc.Decode196,
	"fec/trellis.Decode": trellis.Decode,
	"fec/trellis.Encode": trellis.Encode,
	"layer2/framer.Search": func(stream [600]bit.Bit) (framer.Match, bool) {
		return framer.Search(stream[:], 0, -1)
	},
	"layer2/framer.Framer.Write": func(stream [1200]bit.Bit) []framer.Burst {
		return framer.New(framer.Config{}).Write(stream[:]...)
	},
	"layer2.ShortLCAssembler.AddFragment": func(payload [layer2.CACHPayloadBits]bit.Bit) (bool, error) {
		var a layer2.ShortLCAssembler
		return a.AddFragment(enums.FirstFragmentLC, payload)
	},
	"layer2.VoiceTransmitter.AddSlots": func(slots ...[72]bit.Bit) ([][33]byte, error) {
		return voiceTransmitter().AddSlots(slots...)
	},
	"layer2.VoiceTransmitter.AddFrames": func(frames ...[49]bit.Bit) ([][33]byte, error) {
		return voiceTransmitter().AddFrames(frames...)
	},
	"crc.CalculateCRC7":                                        crc.CalculateCRC7,
	"crc.CalculateCRC8":                                        crc.CalculateCRC8,
	"crc.CalculateCRC9":                                        crc.CalculateCRC9,
	"crc.CheckCRC7":                                            crc.CheckCRC7,
	"crc.CheckCRC8":                                            crc.CheckCRC8,
	"crc.CheckCRC9":                                            crc.CheckCRC9,
	"fec/bptc.DecodeCACH":                                      bptc.DecodeCACH,
	"fec/bptc.DecodeEmbedded":                                  bptc.DecodeEmbedded,
	"fec/bptc.DecodeEmbeddedStrict":                            bptc.DecodeEmbeddedStrict,
	"fec/bptc.DecodeEmbeddedWithCheck":                         bptc.DecodeEmbeddedWithCheck,
	"fec/bptc.DecodeSingleBurst":                               bptc.DecodeSingleBurst,
	"fec/bptc.Encode196":                                       bptc.Encode196,
	"fec/bptc.EncodeCACH":                                      bptc.EncodeCACH,
	"fec/bptc.EncodeEmbedded":                                  bptc.EncodeEmbedded,
	"fec/bptc.EncodeSingleBurst":                               bptc.EncodeSingleBurst,
	"fec/golay.Decode2087":                                     golay.Decode2087,
	"fec/golay.Encode2087":                                     golay.Encode2087,
	"fec/golay.Check2087":                                      golay.Check2087,
	"fec/hamming.Check743":                                     hamming.Check743,
	"fec/hamming.Decode743":                                    hamming.Decode743,
	"fec/hamming.Encode743":                                    hamming.Encode743,
	"fec/quadraticresidue.Check":                               quadraticresidue.Check,
	"fec/quadraticresidue.Decode":                              quadraticresidue.Decode,
	"fec/quadraticresidue.Encode":                              quadraticresidue.Encode,
	"layer2.BurstToDibits":                                     layer2.BurstToDibits,
	"layer2.CACHDeinterleave":                                  layer2.CACHDeinterleave,
	"layer2.CACHInterleave":                                    layer2.CACHInterleave,
	"layer2.DecodeCACH":                                        layer2.DecodeCACH,
	"layer2.DecodeEmbeddedFullLinkControl":                     layer2.DecodeEmbeddedFullLinkControl,
	"layer2.DecodeEmbeddedFullLinkControlStrict":               layer2.DecodeEmbeddedFullLinkControlStrict,
	"layer2.DecodeEmbeddedLC":                                  layer2.DecodeEmbeddedLC,
	"layer2.DecodeEmbeddedLCStrict":                            layer2.DecodeEmbeddedLCStrict,
	"layer2.DecodeRCFromEmbeddedData":                          layer2.DecodeRCFromEmbeddedData,
	"layer2.DecodeShortLCFromFragments":                        layer2.DecodeShortLCFromFragments,
	"layer2.EncodeEmbeddedLC":                                  layer2.EncodeEmbeddedLC,
	"layer2.IsIdleMessage":                                     layer2.IsIdleMessage,
	"layer2.IsNullEmbeddedLC":                                  layer2.IsNullEmbeddedLC,
	"layer2.NewVoiceTransmitter":                               layer2.NewVoiceTransmitter,
	"layer2.ValidateBurstStructure":                            layer2.ValidateBurstStructure,
	"layer2/pdu.CheckMBCLastBlockCRC":                          pdu.CheckMBCLastBlockCRC,
	"layer2/pdu.DecodeAckInboundPDU":                           pdu.DecodeAckInboundPDU,
	"layer2/pdu.DecodeAckInboundPayloadPDU":                    pdu.DecodeAckInboundPayloadPDU,
	"layer2/pdu.DecodeAckOutboundPDU":                          pdu.DecodeAckOutboundPDU,
	"layer2/pdu.DecodeAckOutboundPayloadPDU":                   pdu.DecodeAckOutboundPayloadPDU,
	"layer2/pdu.DecodeAckvitationPDU":                          pdu.DecodeAckvitationPDU,
	"layer2/pdu.DecodeAhoyPDU":                                 pdu.DecodeAhoyPDU,
	"layer2/pdu.DecodeAlohaPDU":                                pdu.DecodeAlohaPDU,
	"layer2/pdu.DecodeBCAPContinuation":                        pdu.DecodeBCAPContinuation,
	"layer2/pdu.DecodeBSOutboundActivationPDU":                 pdu.DecodeBSOutboundActivationPDU,
	"layer2/pdu.DecodeBroadcastTalkgroupVoiceGrantPDU":         pdu.DecodeBroadcastTalkgroupVoiceGrantPDU,
	"layer2/pdu.DecodeCBroadcastPDU":                           pdu.DecodeCBroadcastPDU,
	"layer2/pdu.DecodeCGAPContinuation":                        pdu.DecodeCGAPContinuation,
	"layer2/pdu.DecodeCSBK":                                    pdu.DecodeCSBK,
	"layer2/pdu.DecodeChannelTimingPDU":                        pdu.DecodeChannelTimingPDU,
	"layer2/pdu.DecodeClearPDU":                                pdu.DecodeClearPDU,
	"layer2/pdu.DecodeConfirmedDataHeader":                     pdu.DecodeConfirmedDataHeader,
	"layer2/pdu.DecodeDataHeader":                              pdu.DecodeDataHeader,
	"layer2/pdu.DecodeDefinedDataHeader":                       pdu.DecodeDefinedDataHeader,
	"layer2/pdu.DecodeDuplexPrivateDataGrantPDU":               pdu.DecodeDuplexPrivateDataGrantPDU,
	"layer2/pdu.DecodeDuplexPrivateVoiceGrantPDU":              pdu.DecodeDuplexPrivateVoiceGrantPDU,
	"layer2/pdu.DecodeEmbeddedSignalling":                      pdu.DecodeEmbeddedSignalling,
	"layer2/pdu.DecodeFLCGPSInfo":                              pdu.DecodeFLCGPSInfo,
	"layer2/pdu.DecodeFLCGroupVoice":                           pdu.DecodeFLCGroupVoice,
	"layer2/pdu.DecodeFLCTalkerAliasBlock":                     pdu.DecodeFLCTalkerAliasBlock,
	"layer2/pdu.DecodeFLCTalkerAliasHeader":                    pdu.DecodeFLCTalkerAliasHeader,
	"layer2/pdu.DecodeFLCTerminatorDataLinkControl":            pdu.DecodeFLCTerminatorDataLinkControl,
	"layer2/pdu.DecodeFLCUnitToUnit":                           pdu.DecodeFLCUnitToUnit,
	"layer2/pdu.DecodeFullLinkControl":                         pdu.DecodeFullLinkControl,
	"layer2/pdu.DecodeMBCContinuation":                         pdu.DecodeMBCContinuation,
	"layer2/pdu.DecodeMBCHeader":                               pdu.DecodeMBCHeader,
	"layer2/pdu.DecodeMVAPContinuation":                        pdu.DecodeMVAPContinuation,
	"layer2/pdu.DecodeMaintenancePDU":                          pdu.DecodeMaintenancePDU,
	"layer2/pdu.DecodeMovePDU":                                 pdu.DecodeMovePDU,
	"layer2/pdu.DecodeNegativeAcknowledgementPDU":              pdu.DecodeNegativeAcknowledgementPDU,
	"layer2/pdu.DecodePIHeader":                                pdu.DecodePIHeader,
	"layer2/pdu.DecodePreamblePDU":                             pdu.DecodePreamblePDU,
	"layer2/pdu.DecodePrivateDataGrantMultiItemPDU":            pdu.DecodePrivateDataGrantMultiItemPDU,
	"layer2/pdu.DecodePrivateDataGrantPDU":                     pdu.DecodePrivateDataGrantPDU,
	"layer2/pdu.DecodePrivateVoiceGrantPDU":                    pdu.DecodePrivateVoiceGrantPDU,
	"layer2/pdu.DecodeProprietaryHeader":                       pdu.DecodeProprietaryHeader,
	"layer2/pdu.DecodeProtectPDU":                              pdu.DecodeProtectPDU,
	"layer2/pdu.DecodeRandomAccessPDU":                         pdu.DecodeRandomAccessPDU,
	"layer2/pdu.DecodeRate12Data":                              pdu.DecodeRate12Data,
	"layer2/pdu.DecodeRate1Data":                               pdu.DecodeRate1Data,
	"layer2/pdu.DecodeRate34Data":                              pdu.DecodeRate34Data,
	"layer2/pdu.DecodeRawDataHeader":                           pdu.DecodeRawDataHeader,
	"layer2/pdu.DecodeResponsePacketHeader":                    pdu.DecodeResponsePacketHeader,
	"layer2/pdu.DecodeReverseChannel":                          pdu.DecodeReverseChannel,
	"layer2/pdu.DecodeShortLC":                                 pdu.DecodeShortLC,
	"layer2/pdu.DecodeShortLCActivityUpdate":                   pdu.DecodeShortLCActivityUpdate,
	"layer2/pdu.DecodeShortLCCSysParms":                        pdu.DecodeShortLCCSysParms,
	"layer2/pdu.DecodeShortLCNullMessage":                      pdu.DecodeShortLCNullMessage,
	"layer2/pdu.DecodeShortLCPSysParms":                        pdu.DecodeShortLCPSysParms,
	"layer2/pdu.DecodeSlotType":                                pdu.DecodeSlotType,
	"layer2/pdu.DecodeStatusPrecodedHeader":                    pdu.DecodeStatusPrecodedHeader,
	"layer2/pdu.DecodeTACT":                                    pdu.DecodeTACT,
	"layer2/pdu.DecodeTalkgroupDataGrantMultiItemPDU":          pdu.DecodeTalkgroupDataGrantMultiItemPDU,
	"layer2/pdu.DecodeTalkgroupDataGrantPDU":                   pdu.DecodeTalkgroupDataGrantPDU,
	"layer2/pdu.DecodeTalkgroupVoiceGrantPDU":                  pdu.DecodeTalkgroupVoiceGrantPDU,
	"layer2/pdu.DecodeUDPIPv4CompressedHeader":                 pdu.DecodeUDPIPv4CompressedHeader,
	"layer2/pdu.DecodeUDTHeader":                               pdu.DecodeUDTHeader,
	"layer2/pdu.DecodeUnconfirmedDataHeader":                   pdu.DecodeUnconfirmedDataHeader,
	"layer2/pdu.DecodeUnifiedSingleBlockData":                  pdu.DecodeUnifiedSingleBlockData,
	"layer2/pdu.DecodeUnitToUnitVoiceServiceAnswerResponsePDU": pdu.DecodeUnitToUnitVoiceServiceAnswerResponsePDU,
	"layer2/pdu.DecodeUnitToUnitVoiceServiceRequestPDU":        pdu.DecodeUnitToUnitVoiceServiceRequestPDU,
	"layer2/pdu.DecodeVNAPContinuation":                        pdu.DecodeVNAPContinuation,
	"layer2/pdu.DecodeVocoder":                                 pdu.DecodeVocoder,
	"layer2/pdu.IsPRFill":                                      pdu.IsPRFill,
	"layer2/pdu.MBCLastBlockCRC":                               pdu.MBCLastBlockCRC,
	"layer2/pdu.SetMBCLastBlockCRC":                            pdu.SetMBCLastBlockCRC,
	"layer3.DecodeCdefParms":                          layer3.DecodeCdefParms,
	"layer3.DecodeServiceOptions":                     layer3.DecodeServiceOptions,
	"layer3.NewCdefParmsFromBits":                     layer3.NewCdefParmsFromBits,
	"vocoder.DecodeAMBE72":                                     vocoder.DecodeAMBE72,
	"vocoder.DecodeVocoderFrame":                               vocoder.DecodeVocoderFrame,
	"vocoder.EncodeAMBE72":                                     vocoder.EncodeAMBE72,
}

func voiceTransmitter() *layer2.VoiceTransmitter {
	return must(layer2.NewVoiceTransmitter(groupLC(enums.StandardizedFID), 1, layer2.SyncSourceMS, [72]bit.Bit{}))
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
			if rel == "bit" || rel == filepath.Join("vocoder", "codec2") {
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
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Bit" {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "bit" {
					found = true
				}
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
			if name == "layer2.ValidateBurstStructure" {
				continue // it reports the values that are not 0 or 1
			}
			if raw != nil && masked != nil && !reflect.DeepEqual(raw, masked) {
				t.Errorf("%s: %s values give %v, their low bits give %v", name, p.name, raw, masked)
				break
			}
		}
	}
}
