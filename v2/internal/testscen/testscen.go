// Package testscen draws random, valid DMR test scenarios for the behavior
// tests of the module, so a bar cannot hold only for one fixed payload. It
// draws from the seeds of package testseed, and builds every burst with the
// encoders of the module.
package testscen

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MaxAddress is the highest individual or talkgroup address of a call. ETSI
// TS 102 361-2 clause 7.2.18 and Annex A reserve 0xFFFCE0 to 0xFFFFFF for
// gateways and all-call addresses.
const MaxAddress = 0xFFFCDF

// Address returns a random individual or talkgroup address of a call, 1 to
// MaxAddress.
func Address(g *testseed.Gen) uint32 { return 1 + uint32(g.IntN(MaxAddress)) }

// ColorCode returns a random color code.
func ColorCode(g *testseed.Gen) enums.ColorCode { return enums.ColorCode(g.IntN(16)) }

// Scenario is one random draw of the content of a transmission.
type Scenario struct {
	Source, Dest, BS uint32
	Group            bool
	ColorCode        enums.ColorCode
	VoiceSeed        uint64
	MessageSeed      uint64
}

// Draw returns a random scenario.
func Draw(g *testseed.Gen) Scenario {
	s := Scenario{Source: Address(g), Dest: Address(g), BS: Address(g), Group: g.IntN(2) == 0, ColorCode: ColorCode(g), MessageSeed: g.Uint64()}
	for s.VoiceSeed == 0 {
		s.VoiceSeed = g.Uint64()
	}
	return s
}

// Of returns the scenario drawn from seed.
func Of(seed uint64) Scenario { return Draw(testseed.Of(seed)) }

// Apply returns cfg with the addresses, the voice pattern and the message
// of s. The message keeps the length of cfg.Message, so the bursts of a
// sequence keep their count and kinds. The call type, the color code and
// the layout of cfg stay.
func (s Scenario) Apply(cfg rftx.SeqConfig) rftx.SeqConfig {
	cfg.Source, cfg.Dest, cfg.BS, cfg.VoiceSeed = s.Source, s.Dest, s.BS, s.VoiceSeed
	cfg.Message = string(testseed.Of(s.MessageSeed).Bytes(len(cfg.Message)))
	return cfg
}

// ApplyAll returns Apply of cfg with the call type and the color code of s
// too.
func (s Scenario) ApplyAll(cfg rftx.SeqConfig) rftx.SeqConfig {
	cfg = s.Apply(cfg)
	cfg.Group, cfg.ColorCode = s.Group, s.ColorCode
	return cfg
}

// Config returns a random rftx.SeqConfig on the layout of cfg.
func Config(g *testseed.Gen, cfg rftx.SeqConfig) rftx.SeqConfig { return Draw(g).Apply(cfg) }

// CSBK returns a random standard or Motorola CSBK burst with color code cc
// and the SYNC of src, built by the encoders of the module.
func CSBK(g *testseed.Gen, cc enums.ColorCode, src layer2.SyncSource) (layer2.RawBurst, error) {
	var c pdu.CSBK
	var err error
	switch g.IntN(4) {
	case 0:
		c, err = pdu.NewBSOutboundActivation(Address(g), Address(g))
	case 1:
		c = pdu.CSBK{LastBlock: true}
		err = c.SetPreamble(pdu.PreamblePDU{Data: g.IntN(2) == 0, Group: g.IntN(2) == 0, CSBKBlocksToFollow: byte(g.IntN(256)), TargetAddress: Address(g), SourceAddress: Address(g)})
	case 2:
		c = pdu.CSBK{LastBlock: true}
		err = c.SetMotorolaRadioCheck(pdu.MotorolaRadioCheckPDU{Answer: g.IntN(2) == 0, RequesterAddress: Address(g), CheckedAddress: Address(g)})
	default:
		c = pdu.CSBK{LastBlock: true}
		err = c.SetMotorolaCallAlert(pdu.MotorolaCallAlertPDU{TargetAddress: Address(g), SourceAddress: Address(g)})
	}
	if err != nil {
		return layer2.RawBurst{}, err
	}
	return layer2.BuildBurst(&c, cc, src)
}
