//go:build !tinygo

package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	trellis34 "github.com/USA-RedDragon/dmrgo/v2/internal/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const discDefaultTier = Tier1

func (r *discRx) addSoft(opt *options) error {
	r.addExt(opt)
	return nil
}

func (r *discRx) writeIQ(x []int16) { r.write(x) }

func setExtOn() {}

func setTxHostOn() {}

func txHostOn() bool { return true }

func (r *leanCore) hasExt() bool { return r.ext != nil }

func (r *leanCore) isReplaying() bool { return r.replay != replayNone }

func (r *leanCore) isReplayDry() bool { return r.replay == replayDry }

func (r *leanCore) earlyOK(b *Burst) bool { return checkedData(b) }

// vouched reports whether the coding of b vouches for it when its payload
// started before a large tune: a voice burst goes on to the AMBE FEC and its
// frame flags; a data burst needs a clean slot type and a payload that is a
// codeword with no bit corrected, with its CRC passing where the type has
// one. A rate 1 block has no code and never qualifies.
func (r *leanCore) vouched(b *Burst) bool {
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	if b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice {
		return true
	}
	if !known || kind != layer2.KindData {
		return false
	}
	d, _ := b.Decoded()
	st := d.SlotType()
	if st.FEC.Uncorrectable || st.FEC.ErrorsCorrected != 0 {
		return false
	}
	if st.DataType == enums.DataTypeRate1 {
		return false
	}
	if st.DataType == enums.DataTypeRate34 {
		_, res := trellis34.Decode(d.Info())
		return !res.Uncorrectable && res.ErrorsCorrected == 0
	}
	info, _, res := bptc.Decode196(d.Info())
	if res.Uncorrectable || res.ErrorsCorrected != 0 {
		return false
	}
	ok, checked := bptcCheck(st.DataType, &info)
	return ok || !checked
}
