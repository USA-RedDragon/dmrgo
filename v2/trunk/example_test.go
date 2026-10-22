package trunk_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/trunk/auth"
)

func ExampleFollower() {
	var plan trunk.ChannelPlan
	_ = plan.AddRule(trunk.ChannelRule{First: 1, Last: 200, RXHz: 451_000_000, StepHz: 12_500})
	f, _ := trunk.NewFollower(&trunk.FollowerConfig{Plan: &plan})
	out := make([]trunk.Output, 0, trunk.MaxOutputs)

	code, _ := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 1, Site: 2}.Pack()
	aloha, _ := layer2.BuildBurst(&pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{SysIdentCode: code}}, 1, layer2.SyncSourceBS)
	grant, _ := layer2.BuildBurst(&pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: 17, LogicalChannel: true, TargetAddress: 9, SourceAddress: 3120001}}, 1, layer2.SyncSourceBS)

	now := time.Duration(0)
	for _, raw := range []layer2.RawBurst{aloha, grant} {
		out = f.Burst(now, &raw, layer2.Hint{}, nil, out[:0])
		for _, o := range out {
			switch o.Kind {
			case trunk.OutputSite:
				s := f.Site()
				fmt.Println("site", s.SysCode.Net, s.SysCode.Site)
			case trunk.OutputGrant:
				fmt.Println("grant to talkgroup", o.Grant.Target, "from", o.Grant.Source)
			case trunk.OutputTune:
				fmt.Println("tune to channel", o.Tune.Channel, o.Tune.Slot, o.Tune.RXHz, "Hz")
			default:
			}
		}
		now += 30 * time.Millisecond
	}
	// Output:
	// site 1 2
	// grant to talkgroup 9 from 3120001
	// tune to channel 17 TS2 451200000 Hz
}

// ExampleMS_AuthResponse answers the authentication challenge of a TSCC
// with the key of the MS, as in ETSI TS 102 361-4 V1.12.1 Table 6.15.
func ExampleMS_AuthResponse() {
	key := [auth.KeyLen]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	ms, _ := trunk.NewMS(&trunk.MSConfig{Address: 3120001, NoRegistration: true})
	out := make([]trunk.Output, 0, trunk.MaxOutputs)

	aloha, _ := layer2.BuildBurst(&pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{}}, 1, layer2.SyncSourceBS)
	ahoy, _ := layer2.BuildBurst(&pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAhoy, AhoyPDU: &pdu.AhoyPDU{
		ServiceKind: uint8(enums.ServiceKindRegistration), TargetAddress: 3120001, SourceAddress: 0x7A17C0}}, 1, layer2.SyncSourceBS)

	now := time.Duration(0)
	for _, raw := range []layer2.RawBurst{aloha, ahoy} {
		out = ms.Burst(now, &raw, layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
		for _, o := range out {
			if o.Kind != trunk.OutputAuthChallenge {
				continue
			}
			out = ms.AuthResponse(now, auth.Response(o.Challenge, key, 0), out[:0])
			b, _ := layer2.NewBurstFromBytes(out[0].Burst)
			c, ok := b.Data.(*pdu.CSBK)
			if !ok {
				break
			}
			ack := c.AckInboundPDU
			fmt.Printf("challenge %06X, response %06X from %d\n", o.Challenge, ack.TargetAddress, ack.AdditionalInfo)
			break
		}
		now += 60 * time.Millisecond
	}
	// Output:
	// challenge 7A17C0, response E8CA1D from 3120001
}
