//go:build tinygo

package allocs_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type tinyCallerState struct {
	builder *packet.Builder
	asm     *packet.Assembler
	tx      *layer2.VoiceTransmitter
	bursts  []layer2.RawBurst
	lc      pdu.FullLinkControl
	csbk    pdu.CSBK
	header  pdu.DataHeader
	block   pdu.Rate12Data
	pkt     packet.Packet
	opts    layer2.VoiceTransmitterOptions
	err     error
}

func tinyCallerOps(t *testing.T) (stored, local []tinyOp) {
	t.Helper()
	s := &tinyCallerState{
		builder: packet.NewBuilder(4),
		asm:     packet.NewAssembler(4),
		bursts:  make([]layer2.RawBurst, 0, 8),
		lc:      tinyOK(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{})),
	}
	s.lc.BurstDataType = enums.DataTypeVoiceLCHeader
	s.opts = layer2.VoiceTransmitterOptions{LC: s.lc, ColorCode: 1, Silence: vocoder.AMBESilenceSlot()}
	s.tx = tinyOK(layer2.NewVoiceTransmitter(&s.opts))
	s.csbk = pdu.DecodeCSBK(tinyOK(s.csbk.Encode()))
	s.header = tinyUnconfirmed(pdu.UnconfirmedDataHeader{SAP: enums.SAPIPBasedPacketData, LLIDDestination: 9, LLIDSource: 8, BlocksToFollow: 1, FullMessageFlag: true})
	s.pkt = tinyOK(s.builder.Segment(s.header, []byte("Hello"), enums.DataTypeRate12))
	stored = []tinyOp{
		{"layer2.BuildBurst of a stored CSBK", func() { _, s.err = layer2.BuildBurst(&s.csbk, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildBurst of a stored Full LC", func() { _, s.err = layer2.BuildBurst(&s.lc, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildBurst of a stored data header", func() { _, s.err = layer2.BuildBurst(&s.header, 1, layer2.SyncSourceBS) }},
		{"packet.AppendBursts of a stored Packet", func() {
			s.bursts, s.err = packet.AppendBursts(s.bursts[:0], &s.pkt, 1, layer2.SyncSourceMS)
		}},
		{"layer2.VoiceTransmitter.Reset with stored options", func() { s.err = s.tx.Reset(&s.opts) }},
		{"packet.Assembler.Add of a stored header and block", func() {
			_, _, s.err = s.asm.Add(&s.header)
			_, _, s.err = s.asm.Add(&s.block)
		}},
	}
	local = []tinyOp{
		{"layer2.BuildBurst of a local CSBK", func() {
			c := s.csbk
			_, s.err = layer2.BuildBurst(&c, 1, layer2.SyncSourceBS)
		}},
		{"packet.AppendBursts of a local Packet", func() {
			p := s.pkt
			s.bursts, s.err = packet.AppendBursts(s.bursts[:0], &p, 1, layer2.SyncSourceMS)
		}},
		{"layer2.VoiceTransmitter.Reset with local options", func() {
			o := s.opts
			s.err = s.tx.Reset(&o)
		}},
		{"packet.Assembler.Add of a local block", func() {
			b := s.block
			_, _, s.err = s.asm.Add(&b)
		}},
	}
	return stored, local
}

// TestTinyGoCallerStorage holds the rule of docs/allocation.md and the root doc:
// on TinyGo a value whose address the caller passes allocates when it is
// a local of the caller, and does not when it is in storage that outlives
// the call.
func TestTinyGoCallerStorage(t *testing.T) {
	stored, local := tinyCallerOps(t)
	for _, op := range stored {
		if b, n := tinyAlloc(op.run); b != 0 {
			t.Errorf("%s: %.0f bytes, %.1f allocs per call, want 0", op.name, b, n)
		}
	}
	for _, op := range local {
		b, _ := tinyAlloc(op.run)
		t.Logf("%s: %.0f bytes per call", op.name, b)
	}
}
