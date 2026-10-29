package trunk_test

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/trunk/trunkauth"
)

// ExampleFollower reads the site from an Aloha and tunes to the
// channel of a talkgroup voice grant.
func ExampleFollower() {
	var plan trunk.ChannelPlan
	_ = plan.AddRule(trunk.ChannelRule{First: 1, Last: 200, RXHz: 451_000_000, StepHz: 12_500})
	f, _ := trunk.NewFollower(&trunk.FollowerConfig{Plan: &plan})
	out := make([]trunk.Output, 0, trunk.MaxOutputs)

	code, _ := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 1, Site: 2}.Pack()
	alohaCSBK := pdu.CSBK{LastBlock: true}
	_ = alohaCSBK.SetAloha(pdu.AlohaPDU{SysIdentCode: code})
	grantCSBK := pdu.CSBK{LastBlock: true}
	_ = grantCSBK.SetTalkgroupVoiceGrant(pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 17, LogicalChannel: true, TargetAddress: 9, SourceAddress: 3120001})
	aloha, _ := layer2.BuildBurst(&alohaCSBK, 1, layer2.SyncSourceBS)
	grant, _ := layer2.BuildBurst(&grantCSBK, 1, layer2.SyncSourceBS)

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
	key := [trunkauth.KeyLen]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	ms, _ := trunk.NewMS(&trunk.MSConfig{Address: 3120001, NoRegistration: true})
	out := make([]trunk.Output, 0, trunk.MaxOutputs)

	alohaCSBK := pdu.CSBK{LastBlock: true}
	_ = alohaCSBK.SetAloha(pdu.AlohaPDU{})
	ahoyCSBK := pdu.CSBK{LastBlock: true}
	_ = ahoyCSBK.SetAhoy(pdu.AhoyPDU{ServiceKind: uint8(enums.ServiceKindRegistration), TargetAddress: 3120001, SourceAddress: 0x7A17C0})
	aloha, _ := layer2.BuildBurst(&alohaCSBK, 1, layer2.SyncSourceBS)
	ahoy, _ := layer2.BuildBurst(&ahoyCSBK, 1, layer2.SyncSourceBS)

	now := time.Duration(0)
	for _, raw := range []layer2.RawBurst{aloha, ahoy} {
		out = ms.Burst(now, &raw, layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
		for _, o := range out {
			if o.Kind != trunk.OutputAuthChallenge {
				continue
			}
			out = ms.AuthResponse(now, trunkauth.Response(o.Challenge, key, 0), out[:0])
			b, _ := layer2.NewBurstFromBytes(out[0].Burst)
			c, ok := b.Data.(*pdu.CSBK)
			if !ok {
				break
			}
			ack, ok := c.AsAckInbound()
			if !ok {
				break
			}
			fmt.Printf("challenge %06X, response %06X from %d\n", o.Challenge, ack.TargetAddress, ack.AdditionalInfo)
			break
		}
		now += 60 * time.Millisecond
	}
	// Output:
	// challenge 7A17C0, response E8CA1D from 3120001
}

// ExampleMS registers an MS on a TSCC, makes a talkgroup voice call by
// random access, follows the grant to the payload channel, and ends the
// call. newServiceSite and run, in example_services_test.go, carry the
// bursts between the TSCC and the MS on a 30 ms slot clock.
func ExampleMS() {
	s := newServiceSite(&trunk.TSCCConfig{}, &trunk.MSConfig{Address: 3120001, Seed: 1})
	s.onMS = func(_ int, o *trunk.Output) {
		switch o.Kind {
		case trunk.OutputRegistered:
			fmt.Println(o.Kind)
		case trunk.OutputCallEnd:
			fmt.Println(o.Kind, o.Reason)
		case trunk.OutputGrant:
			fmt.Println(o.Kind, "talkgroup", o.Grant.Target, "from", o.Grant.Source)
		case trunk.OutputTune:
			fmt.Println(o.Kind, "channel", o.Tune.Channel, o.Tune.Slot)
		default:
		}
	}
	s.run(3 * time.Second)
	fmt.Println(s.ms[0].State(), "registered", s.ms[0].Registered())
	if err := s.ms[0].Call(s.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
		fmt.Println(err)
		return
	}
	s.run(time.Second)
	fmt.Println(s.ms[0].State())
	s.take(0, s.ms[0].EndCall(s.now, s.mout[:0]))
	s.run(time.Second)
	fmt.Println(s.ms[0].State())
	// Output:
	// Registered
	// Control registered true
	// Grant talkgroup 9 from 3120001
	// Tune channel 10 TS1
	// Payload
	// CallEnd Maintenance
	// Tune channel 0 Unknown
	// Control
}

// ExampleTSCC runs a single site with one payload channel, and reports
// the registration of an MS and the start and end of its call.
func ExampleTSCC() {
	s := newServiceSite(&trunk.TSCCConfig{}, &trunk.MSConfig{Address: 3120001, Seed: 1})
	s.onTSCC = func(o *trunk.TSCCOutput) {
		switch o.Kind {
		case trunk.TSCCRegistered:
			fmt.Println(o.Kind, o.Address)
		case trunk.TSCCCallStart:
			fmt.Println(o.Kind, "talkgroup", o.Grant.Target, "from", o.Grant.Source, "on channel", o.Grant.Channel, o.Grant.Slot)
		case trunk.TSCCCallEnd:
			fmt.Println(o.Kind, o.Reason)
		default:
		}
	}
	s.run(3 * time.Second)
	_ = s.ms[0].Call(s.now, true, 9, layer3.VoiceServiceOptions{})
	s.run(time.Second)
	s.take(0, s.ms[0].EndCall(s.now, s.mout[:0]))
	s.run(time.Second)
	fmt.Println("registered", s.tscc.Registered(3120001))
	// Output:
	// Registered 3120001
	// CallStart talkgroup 9 from 3120001 on channel 10 TS1
	// CallEnd Maintenance
	// registered true
}

// ExampleCapacityPlusFollower reads the rest channel beacons of a Linked
// Capacity Plus site from a recording, reports the rest channel and the
// site number, and hunts again when the beacons stop.
func ExampleCapacityPlusFollower() {
	file, err := os.ReadFile("testdata/capplus_lcp_beacon.txt")
	if err != nil {
		fmt.Println(err)
		return
	}
	f, err := trunk.NewCapacityPlusFollower(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	var now time.Duration
	for line := range strings.Lines(string(file)) {
		cols := strings.Fields(line)
		if len(cols) != 3 || strings.HasPrefix(cols[0], "#") {
			continue
		}
		ms, _ := strconv.ParseFloat(cols[0], 64)
		cach, _ := hex.DecodeString(cols[1])
		raw, _ := hex.DecodeString(cols[2])
		if len(cach) != 3 || len(raw) != 33 {
			continue
		}
		now = time.Duration(ms * float64(time.Millisecond))
		out = f.Burst(now, (*layer2.RawBurst)(raw), layer2.Hint{}, (*[3]byte)(cach), out[:0])
		for _, o := range out {
			lsn, _ := f.RestLSN()
			fmt.Println(o.Kind, "rest LSN", lsn, f.State())
		}
	}
	site, _ := f.SiteNumber()
	fmt.Println("site", site)
	at, _ := f.Deadline()
	for _, o := range f.Tick(at, out[:0]) {
		fmt.Println(o.Kind, "after", at-now, f.State())
	}
	// Output:
	// Site rest LSN 6 Control
	// site 3
	// Lost after 5s Hunting
}
