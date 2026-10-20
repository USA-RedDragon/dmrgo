//go:build !tinygo

package layer2_test

import (
	"fmt"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func buildAndEncode(d layer2.Data) error {
	r, err := layer2.BuildBurst(d, 1, layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	b := layer2.Burst{
		Kind:        layer2.KindData,
		SyncPattern: enums.SyncBSSourcedData,
		SlotType:    r.SlotType(),
		Data:        d,
	}
	again, err := b.Encode()
	if err != nil {
		return err
	}
	if again != r {
		return fmt.Errorf("Burst.Encode = %x, want %x", again, r)
	}
	return nil
}

func onStack[T any, P interface {
	*T
	layer2.Data
}](seed T) (string, func() error) {
	return fmt.Sprintf("%T", seed), func() error {
		v := seed
		return buildAndEncode(P(&v))
	}
}

func TestAllocsBuildBurst(t *testing.T) {
	lc := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.MotorolaServiceOptions()))
	term := lc
	term.BurstDataType = enums.DataTypeTerminatorWithLC
	header := pdu.DataHeader{
		Format:                pdu.FormatUnconfirmed,
		UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{Group: true, SAP: 4, BlocksToFollow: 3},
	}
	mbc := pdu.MBCHeader{
		Opcode:                 pdu.CSBKTalkgroupVoiceGrant,
		TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF},
	}
	type pduCase struct {
		name  string
		build func() error
	}
	var cases []pduCase
	add := func(name string, f func() error) { cases = append(cases, pduCase{name, f}) }
	add(onStack(lc))
	add(onStack(term))
	add(onStack(must(pdu.NewBSOutboundActivation(310123, 3191868))))
	add(onStack(pdu.PIHeader{AlgorithmID: 1, KeyID: 2, MessageIndicator: 3, Destination: 91}))
	add(onStack(header))
	add(onStack(mbc))
	add(onStack(pdu.MBCContinuation{LastBlock: true}))
	add(onStack(pdu.CGAPContinuation{LastBlock: true}))
	add(onStack(pdu.MVAPContinuation{LastBlock: true}))
	add(onStack(pdu.BCAPContinuation{LastBlock: true}))
	add(onStack(pdu.VNAPContinuation{LastBlock: true}))
	add(onStack(pdu.UnifiedSingleBlockData{Payload: [10]byte{1, 2}}))
	add(onStack(pdu.Rate12Data{Data: [12]byte{1, 2, 3}}))
	add(onStack(pdu.Rate34Data{Data: [18]byte{4, 5, 6}}))
	add(onStack(pdu.Rate1Data{Data: [24]byte{7, 8, 9}}))
	add(onStack(pdu.PRFill{}))
	for _, c := range cases {
		if err := c.build(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if n := testing.AllocsPerRun(100, func() { _ = c.build() }); n != 0 {
			t.Errorf("%s: %.1f allocations, want 0", c.name, n)
		}
	}
}
