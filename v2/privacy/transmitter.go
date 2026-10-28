package privacy

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// Service is the radio service of a transmitting station. It sets
// whether the station may encrypt: the amateur service may not, under ITU
// Radio Regulations Article 25.2A and national rules such as 47 CFR
// 97.113(a)(4) in the United States. The rule holds in every region, so a
// Service names no region. Receiving is allowed in every service. It is
// dmrgo.Service, the type that package rf also takes.
type Service = dmrgo.Service

// The Service values.
const (
	// ServiceUnknown is a service that the caller did not state.
	// NewTransmitter returns a *dmrgo.ConfigError for it.
	ServiceUnknown = dmrgo.ServiceUnknown
	// ServiceAmateur is the amateur or amateur-satellite service, which
	// forbids messages encoded to obscure their meaning. NewTransmitter
	// returns ErrAmateur for it.
	ServiceAmateur = dmrgo.ServiceAmateur
	// ServiceLandMobile is a licensed land mobile service, such as private,
	// business or public safety PMR.
	ServiceLandMobile = dmrgo.ServiceLandMobile
	// ServiceLicenseFree is a licence-exempt service, such as PMR446.
	ServiceLicenseFree = dmrgo.ServiceLicenseFree
)

// TransmitterConfig configures NewTransmitter.
type TransmitterConfig struct {
	// Key is the key of the calls.
	Key *Key
	// Service is the radio service of the station. It must be stated, and
	// it must not be ServiceAmateur.
	Service Service
}

// Transmitter applies the privacy of one key to the calls of a
// voice.Transmitter. For each call, Start it with a new MI, put its
// PIHeader in voice.CallOptions.PIHeader, set the Privacy service option
// and FID 0x10 in the voice.Call, and pass every Output of the
// voice.Transmitter through Output before it goes on air. Make it with
// NewTransmitter. A Transmitter is not safe for concurrent use and must not
// be copied; go vet reports a copy.
type Transmitter struct {
	key     *Key
	mi      uint32
	started bool
	voice   bool
	burst   int
	stream  stream
	dataRC  arc4
	dataKey [9]byte
	noCopy  noCopy
}

// NewTransmitter returns a Transmitter for cfg. It returns ErrAmateur when
// cfg.Service is ServiceAmateur, whatever the key, and a *dmrgo.ConfigError
// for ServiceUnknown or a Service out of range, so that no encrypting
// Transmitter exists in the amateur service or in a service not stated. It
// returns dmrgo.ErrNilArgument for a nil cfg or a nil cfg.Key, and a
// *dmrgo.ConfigError for a Key that is not made with its constructor. It
// allocates the Transmitter.
func NewTransmitter(cfg *TransmitterConfig) (*Transmitter, error) {
	if cfg == nil {
		return nil, dmrgo.ErrNilArgument
	}
	switch cfg.Service {
	case ServiceAmateur:
		return nil, ErrAmateur
	case ServiceLandMobile, ServiceLicenseFree:
	case ServiceUnknown:
		return nil, errService
	default:
		return nil, errService
	}
	k := cfg.Key
	if k == nil {
		return nil, dmrgo.ErrNilArgument
	}
	switch k.alg {
	case AlgorithmARC4, AlgorithmBasic:
	case AlgorithmAES128, AlgorithmAES256:
		if k.block == nil {
			return nil, errAESKey
		}
	default:
		return nil, errAESKey
	}
	return &Transmitter{key: k}, nil
}

// Start begins a call with the message indicator mi. Use a new random mi
// for each call, such as 4 octets from crypto/rand: a repeated mi repeats
// the keystream. A Basic Privacy key ignores mi.
func (t *Transmitter) Start(mi uint32) {
	if t == nil {
		return
	}
	t.mi, t.started, t.voice, t.burst = mi, true, false, 0
	t.stream.start(t.key, mi)
}

// PIHeader returns the PI header of the call that Start began, for the
// destination address target. ok is false for a Basic Privacy key, which
// sends no PI header, and before Start. A target over 24 bits fails
// PIHeader.Encode.
func (t *Transmitter) PIHeader(target uint32) (h pdu.PIHeader, ok bool) {
	if t == nil || !t.started || t.key.alg == AlgorithmBasic {
		return pdu.PIHeader{}, false
	}
	return pdu.PIHeader{
		AlgorithmID:      uint8(t.key.alg),
		MFID:             enums.FIDMotorola,
		KeyID:            t.key.id,
		MessageIndicator: t.mi,
		Destination:      target,
	}, true
}

// Output applies the privacy to o in place when it is a voice burst: the
// three AMBE+2 frames of the burst, fill and silence frames too. It leaves
// other outputs as they are. It returns ErrNotStarted before Start,
// dmrgo.ErrNilArgument for a nil o, and the errors of
// layer2.RawBurst.SetAMBEFrames, which then change nothing.
func (t *Transmitter) Output(o *voice.Output) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}
	if o == nil {
		return dmrgo.ErrNilArgument
	}
	if !t.started {
		return ErrNotStarted
	}
	if o.Kind != voice.OutputBurst || o.Hint.Kind != layer2.KindVoice {
		return nil
	}
	b := t.burst
	if t.voice {
		b++
	}
	if idx, ok := o.Hint.VoiceBurst.Index(); ok {
		for b%6 != idx {
			b++
		}
	}
	frames, _ := o.Burst.AMBEFrames()
	for i := range frames {
		t.stream.apply(b*layer2.VoiceSlotsPerBurst+i, &frames[i], false)
	}
	if err := o.Burst.SetAMBEFrames(frames); err != nil {
		return err
	}
	t.voice, t.burst = true, b
	return nil
}
