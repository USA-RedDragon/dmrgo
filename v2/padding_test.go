package dmrgo_test

import (
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// paddingCase is a field of bits that is not a whole number of bytes: run
// decodes the random input p, whose padding bits are 0, and padded, the same
// bits with padding bits set, and returns both results and the bytes that
// an encode of the first writes.
type paddingCase struct {
	name string
	bits int
	run  func(p, padded []byte) (plain, set any, enc []byte)
}

// TestPaddingBits checks the padding rule of the packed API: a decode
// ignores the low bits of the last byte, and an encode writes them as 0.
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
		{"layer2.ShortLCAssembler.AddFragment", 17, func(p, q []byte) (any, any, []byte) {
			var a, b layer2.ShortLCAssembler
			lcss := []enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
			for _, l := range lcss {
				_, _ = a.AddFragment(l, [3]byte(p))
				_, _ = b.AddFragment(l, [3]byte(q))
			}
			sa, errA := a.Complete()
			sb, errB := b.Complete()
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
