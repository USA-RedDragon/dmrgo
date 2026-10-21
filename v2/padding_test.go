package dmrgo_test

import (
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

type paddingCase struct {
	name string
	bits int
	run  func(p, padded []byte) (plain, set any, enc []byte)
}

func TestPaddingBits(t *testing.T) {
	t.Parallel()

	cases := []paddingCase{
		{"pdu.TACT", 7, func(p, q []byte) (any, any, []byte) {
			a := pdu.DecodeTACT([1]byte(p))
			e, _ := a.Encode()
			return a, pdu.DecodeTACT([1]byte(q)), e[:]
		}},
		{"pdu.ReverseChannel", 11, func(p, q []byte) (any, any, []byte) {
			a := pdu.DecodeReverseChannel([2]byte(p))
			e, _ := a.Encode()
			return a, pdu.DecodeReverseChannel([2]byte(q)), e[:]
		}},
		{"pdu.SlotType", 20, func(p, q []byte) (any, any, []byte) {
			a := pdu.DecodeSlotType([3]byte(p))
			e, _ := a.Encode()
			return a, pdu.DecodeSlotType([3]byte(q)), e[:]
		}},
		{"pdu.ShortLC", 36, func(p, q []byte) (any, any, []byte) {
			var a, b pdu.ShortLC
			var pa, pb pdu.ShortLCPayloads
			pdu.DecodeShortLCInto(&a, &pa, [5]byte(p))
			pdu.DecodeShortLCInto(&b, &pb, [5]byte(q))
			e, _ := a.Encode()
			return a.String(), b.String(), e[:]
		}},
		{"layer3.CdefParms", 58, func(p, q []byte) (any, any, []byte) {
			a := layer3.DecodeCdefParms([8]byte(p))
			e, _ := a.Encode()
			return a, layer3.DecodeCdefParms([8]byte(q)), e[:]
		}},
		{"layer2.ShortLCAssembler.Add", 17, func(p, q []byte) (any, any, []byte) {
			var a, b layer2.ShortLCAssembler
			var sa, sb pdu.ShortLC
			var errA, errB error
			lcss := []enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
			for _, l := range lcss {
				sa, _, errA = a.Add(layer2.CACH{TACT: pdu.TACT{LCSS: l}, Payload: [3]byte(p)})
				sb, _, errB = b.Add(layer2.CACH{TACT: pdu.TACT{LCSS: l}, Payload: [3]byte(q)})
			}
			c := layer2.CACH{Payload: [3]byte(p)}
			e, _ := c.Encode()
			back := layer2.DecodeCACH(e)
			return [2]any{sa.String(), errA}, [2]any{sb.String(), errB}, back.Payload[:]
		}},
	}
	r := rand.New(rand.NewPCG(1, 2))
	for _, c := range cases {
		n := (c.bits + 7) / 8
		pad := byte(1)<<(8*n-c.bits) - 1
		for range 2000 {
			p := make([]byte, n)
			for i := range p {
				p[i] = byte(r.Uint32())
			}
			p[n-1] &^= pad
			q := append([]byte(nil), p...)
			q[n-1] |= byte(r.Uint32())&pad | 1
			clr, set, enc := c.run(p, q)
			if clr != set {
				t.Fatalf("%s: input %x decodes to %v, and with padding %x to %v", c.name, p, clr, q, set)
			}
			if len(enc) != n || enc[n-1]&pad != 0 {
				t.Fatalf("%s: encode of %x is %x, whose padding bits are not 0", c.name, p, enc)
			}
		}
	}
}
