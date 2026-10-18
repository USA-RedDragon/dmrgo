package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// exactKinds is the number of burst kinds of exactBurst: every data type,
// then voice bursts A to F.
const exactKinds = int(enums.DataTypeReserved) + 6

// exactBurst returns a burst of kind i of exactKinds with color code cc and
// random content, its SYNC and voice burst position, and a CACH with a
// valid TACT.
func exactBurst(t *testing.T, rng *rand.Rand, i int, cc enums.ColorCode) (layer2.RawBurst, [3]byte, enums.SyncPattern, enums.VoiceBurstType) {
	t.Helper()
	var r layer2.RawBurst
	var sync enums.SyncPattern
	voice := enums.VoiceBurstUnknown
	if dt := enums.DataType(i); dt < enums.DataTypeReserved {
		switch dt {
		case enums.DataTypeRate34:
			var p [18]byte
			for k := range p {
				p[k] = byte(rng.IntN(256))
			}
			r.SetRate34Payload(p)
		case enums.DataTypeRate1:
			var p [24]byte
			for k := range p {
				p[k] = byte(rng.IntN(256))
			}
			r.SetRate1Payload(p)
		default:
			var p [12]byte
			for k := range p {
				p[k] = byte(rng.IntN(256))
			}
			if softBPTCCheck(dt) != nil {
				if err := pdu.Seal(&p, dt); err != nil {
					t.Fatal(err)
				}
			}
			r.SetBPTCPayload(p)
		}
		if err := r.SetSlotType(pdu.SlotTypeInfo(byte(cc)<<4 | byte(dt)).SlotType()); err != nil {
			t.Fatal(err)
		}
		sync = enums.SyncBSSourcedData
		_ = r.SetSync(sync)
	} else {
		for k := range r {
			r[k] = byte(rng.IntN(256))
		}
		if err := r.SetAMBEFrames(softRandomFrames(rng)); err != nil {
			t.Fatal(err)
		}
		voice = enums.VoiceBurstA + enums.VoiceBurstType(i-int(enums.DataTypeReserved))
		if voice == enums.VoiceBurstA {
			sync = enums.SyncBSSourcedVoice
			_ = r.SetSync(sync)
		} else {
			e := pdu.EMBInfo(rng.IntN(128) << 1)
			if err := e.SetColorCode(cc); err != nil {
				t.Fatal(err)
			}
			if err := r.SetEMB(e.EmbeddedSignalling()); err != nil {
				t.Fatal(err)
			}
		}
	}
	cach := [3]byte{byte(rng.IntN(256)), byte(rng.IntN(256)), byte(rng.IntN(256))}
	c := &softSmallCodes[softTACTCode]
	cw := c.base
	for k := range c.k {
		if rng.IntN(2) != 0 {
			cw ^= c.rows[k]
		}
	}
	for j := range c.n {
		p := c.pos[j]
		cach[p/8] = cach[p/8]&^(0x80>>(p%8)) | byte(cw>>j&1)<<(7-p%8)
	}
	return r, cach, sync, voice
}

// leanHard sets the CACH and Data of b to the hard decisions that
// leanCore.burst makes of the slot of r that starts at ring index 0.
func leanHard(r *leanCore, b *Burst) {
	thr := 2 * r.llrDev
	var acc byte
	for k := range slotSymbols {
		x := int32(r.ring[k])*(discStepHz/llrStepHz) + r.llrOff
		acc <<= 2
		if x < 0 {
			acc |= 2
			x = -x
		}
		if x > thr {
			acc |= 1
		}
		switch {
		case k&3 != 3:
		case k < cachSymbols:
			b.CACH[k/4] = acc
		default:
			b.Data[(k-cachSymbols)/4] = acc
		}
	}
}

// TestLeanRepairRecorded repairs a rate 1/2 data burst that Tier0 detected
// without a bit error, from the symbols that it held. Its front end had
// clipped the outer levels of the SYNC, so that the fit gave a noise
// variance of 0, and every symbol off the outer levels has both levels of
// each pair at the outlier floor. The repair must leave the burst as it is.
func TestLeanRepairRecorded(t *testing.T) {
	t.Parallel()
	ring := [slotSymbols]int8{
		61, -59, 66, 58, -61, 63, -73, -128, 127, 59, -64, -56, 60, 63, 68, 57, 76, 127, -51, 127, 63, 64, 60, 73,
		48, -128, 50, -128, 57, 69, 59, 73, 127, 64, 71, 127, 68, 63, 65, 58, -59, 65, 59, -60, 65, 64, 62, 62,
		68, 127, 127, 65, 64, 61, 65, 61, 63, 58, -59, 62, 56, -128, -128, 127, -128, 37, -128, 127, -128, -128, -128, -128,
		127, 127, 127, -128, -128, 127, 127, -128, 127, 127, -128, 127, -128, -128, 127, 127, -128, 127, 62, -66, -59, -74, -128, 61,
		64, 57, -59, 60, 63, 64, 62, 63, 64, 62, 64, 63, 62, 65, 60, 74, 127, 67, 59, -59, 63, 65, 62, 73,
		127, 71, 127, 68, 60, 64, 62, -72, -49, 127, 66, 64, 63, 70, 47, -128, 57, -56, 57, 73, 57, 73, 127, -70,
	}
	r := &leanCore{llrOff: 8, llrDev: 680, llrK: 1 << 30, llrFloor: -98, anchor: enums.SyncBSSourcedData}
	copy(r.ring[:], ring[:])
	b := Burst{Sync: enums.SyncBSSourcedData, HasCACH: true}
	leanHard(r, &b)
	if p, res := b.Data.BPTCPayload(); p != [12]byte{0x01, 0x0f} || res.ErrorsCorrected != 0 || b.Data.SlotType().DataType != enums.DataTypeRate12 {
		t.Fatalf("recorded burst: %v payload %x, %+v", b.Data.SlotType().DataType, p, res)
	}
	r.repair(&r.fec, &b, 15, true)
	if p, _ := b.Fixed.BPTCPayload(); b.Fixed != b.Data || b.FixedCACH != b.CACH {
		t.Errorf("repair changed the burst: payload %x, Fixed %x, Data %x", p, b.Fixed, b.Data)
	}
}

// TestLeanRepairKeepsExact repairs bursts of every data type and voice
// bursts that the core detected without a bit error, with the symbols at
// their levels or anywhere in their decision regions, and likelihoods from
// a noisy fit to one without noise, with and without the outlier floor,
// and checks that the repair leaves every bit as it is.
func TestLeanRepairKeepsExact(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(5, 6))
	const level = 40
	for _, v := range []float64{1, 30, 1e3, 1e5} {
		for _, floor := range []int32{math.MinInt32 / 2, -98} {
			for _, spread := range []bool{false, true} {
				for i := range 8 * exactKinds {
					cc := enums.ColorCode(rng.IntN(16))
					raw, cach, sync, voice := exactBurst(t, rng, i%exactKinds, cc)
					r := &leanCore{llrDev: level * discStepHz / llrStepHz, llrFloor: floor, anchor: sync}
					r.llrK = int64(8*llrStepHz*llrStepHz*(1<<llrShift)/(2*v) + 0.5)
					for k := range slotSymbols {
						var d byte
						if k < cachSymbols {
							d = cach[k/4] >> (6 - 2*(k%4)) & 3
						} else {
							d = raw[(k-cachSymbols)/4] >> (6 - 2*((k-cachSymbols)%4)) & 3
						}
						q := level + 2*level*int(d&1)
						switch {
						case !spread:
						case d&1 == 0:
							q = rng.IntN(2*level + 1)
						default:
							q = 2*level + 1 + rng.IntN(127-2*level+int(d>>1))
						}
						if d&2 != 0 {
							q = -max(q, 1)
						}
						r.ring[k] = int8(q)
					}
					b := Burst{Sync: sync, Voice: voice, HasCACH: true}
					leanHard(r, &b)
					if b.Data != raw || b.CACH != cach {
						t.Fatalf("kind %d: the symbols do not slice to the burst", i%exactKinds)
					}
					for _, ccOK := range []bool{false, true} {
						r.repair(&r.fec, &b, cc, ccOK)
						if b.Fixed != raw || b.FixedCACH != cach {
							t.Fatalf("kind %d, variance %v, floor %d, spread %v, cc known %v: repair changed the burst\nFixed %x\nData  %x", i%exactKinds, v, floor, spread, ccOK, b.Fixed, raw)
						}
					}
				}
			}
		}
	}
}

// TestSoftRepairKeepsExact decodes bursts of every data type and voice
// bursts from ratios with the signs of their bits at any magnitude from 1,
// as the stages of Tier1 and the repair of Tier2 and Tier3 get them, with
// each of their decoders, and checks that each decode leaves the burst as
// it is.
func TestSoftRepairKeepsExact(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(7, 8))
	d := newSoftDecoder()
	for i := range 20 * exactKinds {
		cc := enums.ColorCode(rng.IntN(16))
		raw, cach, _, voice := exactBurst(t, rng, i%exactKinds, cc)
		mag := func() int16 {
			if rng.IntN(8) == 0 {
				return int16(1 + rng.IntN(32000))
			}
			return int16(1 + rng.IntN(254))
		}
		bits := [softBurstBits]unpacked.Bit(unpacked.Unpack(raw[:]))
		var llr [softBurstBits]int16
		for k, v := range bits {
			llr[k] = mag() * int16(1-2*int(v))
		}
		var cllr [layer2.CACHBits]int16
		cb := cachBits(cach)
		for k, v := range cb {
			cllr[k] = mag() * int16(1-2*int(v))
		}
		for _, ccOK := range []bool{false, true} {
			out := raw
			if voice != enums.VoiceBurstUnknown {
				d.decodeAMBE(&llr, &out)
				if voice != enums.VoiceBurstA {
					d.decodeEMB(&llr, cc, ccOK, &out)
				}
			} else {
				st, _ := d.decodeSlotType(&llr, cc, ccOK, &out)
				switch dt := st.DataType(); dt {
				case enums.DataTypeRate34:
					d.decodeRate34(&llr, &out)
				case enums.DataTypeRate1, enums.DataTypeIdle:
				default:
					check := softBPTCCheck(dt)
					for name, dec := range map[string]func(*[softBurstBits]int16, func([12]byte) bool, *layer2.RawBurst) (softBPTCResult, fec.Result){
						"OSD": d.decodeBPTCOSD, "Chase": d.decodeBPTCChase, "grid": d.decodeBPTCGrid,
					} {
						o := out
						dec(&llr, check, &o)
						if o != raw {
							t.Fatalf("kind %d: %s decode changed the burst\nout %x\nraw %x", i%exactKinds, name, o, raw)
						}
					}
				}
			}
			if out != raw {
				t.Fatalf("kind %d, cc known %v: decode changed the burst\nout %x\nraw %x", i%exactKinds, ccOK, out, raw)
			}
			var tact [layer2.CACHBits]unpacked.Bit
			copy(tact[:], cb[:])
			d.decodeTACT(&cllr, &tact)
			if tact != cb {
				t.Fatalf("kind %d: TACT decode changed the CACH", i%exactKinds)
			}
		}
	}
}
