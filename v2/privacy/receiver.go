package privacy

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// MaxKeys is the most keys that a Receiver holds.
const MaxKeys = 16

// Info is what a Receiver knows about the privacy of the current call. It
// needs no key.
type Info struct {
	// Encrypted reports that the call is encrypted: its LC sets the
	// Privacy service option, or a PI header came.
	Encrypted bool
	// Header reports that a PI header with a good CRC came, so that MFID,
	// Algorithm, KeyID and MI are set.
	Header bool
	// MFID is the FID of the PI header. Algorithm, KeyID and MI follow the
	// layout of FID 0x10; for another FID they hold its octets 0, 2 and 3
	// to 6 as they are.
	MFID enums.FID
	// Algorithm is the algorithm ID of the PI header.
	Algorithm Algorithm
	// KeyID is the key ID of the PI header.
	KeyID uint8
	// MI is the 32-bit message indicator of the PI header.
	MI uint32
	// Decrypting reports that the Receiver holds the key of the call and
	// returns its frames in the clear.
	Decrypting bool
}

// FromPIHeader returns the Info of the PI header h: Encrypted and Header
// set and the fields of h. It returns the zero Info for a nil h and for a
// PI header whose CRC failed.
func FromPIHeader(h *pdu.PIHeader) Info {
	if h == nil || h.FEC.Uncorrectable {
		return Info{}
	}
	return Info{
		Encrypted: true,
		Header:    true,
		MFID:      h.MFID,
		Algorithm: Algorithm(h.AlgorithmID),
		KeyID:     h.KeyID,
		MI:        h.MessageIndicator,
	}
}

// Receiver removes the privacy of the voice calls that a voice.Receiver
// reports. Feed it every event of the voice.Receiver in order. It finds
// the key of a call from the algorithm and key ID of its PI header, or uses
// the Basic Privacy key for a call whose LC sets the Privacy service option
// and that has no PI header before its voice. Make it with
// NewReceiver. A Receiver is not safe for concurrent use and must not be
// copied; go vet reports a copy.
type Receiver struct {
	keys   [MaxKeys]*Key
	nKeys  int
	basic  *Key
	info   Info
	voice  bool
	burst  int
	stream stream
	noCopy noCopy
}

// NewReceiver returns a Receiver without keys. It allocates the Receiver.
func NewReceiver() *Receiver {
	return &Receiver{}
}

// AddKey adds k to the keys of the Receiver. A key with the algorithm and
// key ID of one that the Receiver holds replaces it, and a Basic Privacy key
// replaces the one before. It returns dmrgo.ErrNilArgument for a nil k and
// ErrKeysFull when the Receiver holds MaxKeys other keys. The current call
// keeps the key it has.
func (r *Receiver) AddKey(k *Key) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}
	if k == nil {
		return dmrgo.ErrNilArgument
	}
	if k.alg == AlgorithmBasic {
		r.basic = k
		return nil
	}
	for i := range r.nKeys {
		if r.keys[i].alg == k.alg && r.keys[i].id == k.id {
			r.keys[i] = k
			return nil
		}
	}
	if r.nKeys == len(r.keys) {
		return ErrKeysFull
	}
	r.keys[r.nKeys] = k
	r.nKeys++
	return nil
}

// Reset forgets the current call. It keeps the keys.
func (r *Receiver) Reset() {
	if r == nil {
		return
	}
	r.info, r.voice, r.burst = Info{}, false, 0
	r.stream.stop()
}

// Info returns what the Receiver knows about the current call.
func (r *Receiver) Info() Info {
	if r == nil {
		return Info{}
	}
	return r.info
}

func (r *Receiver) lookup(i *Info) *Key {
	if i.MFID != enums.FIDMotorola {
		return nil
	}
	for _, k := range r.keys[:r.nKeys] {
		if k.alg == i.Algorithm && k.id == i.KeyID {
			return k
		}
	}
	return nil
}

// Event takes the next event e of the voice.Receiver and returns the Info
// of its call. For an EventVoice it writes the three AMBE+2 frames of the
// burst to frames, in the clear when Info.Decrypting is set and as received
// otherwise; in an ARC4 or AES call a frame received as
// vocoder.AMBESilence stays as it is. Frames is not written for another event, or when it is nil. The
// Info of an EventCallEnd is that of the call that ended; the Receiver
// then forgets the call. A nil e changes nothing.
func (r *Receiver) Event(e *voice.Event, frames *[layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame) Info {
	if r == nil {
		return Info{}
	}
	if e == nil {
		return r.info
	}
	switch e.Kind {
	case voice.EventCallStart:
		if e.Call.Options.IsPrivacy {
			r.info.Encrypted = true
		}
	case voice.EventPIHeader:
		r.header(e)
	case voice.EventVoice:
		r.voiceBurst(e, frames)
	case voice.EventCallEnd:
		info := r.info
		r.Reset()
		return info
	case voice.EventEmbedded, voice.EventTalkerAlias, voice.EventSessionStart, voice.EventSessionEnd:
	}
	return r.info
}

func (r *Receiver) header(e *voice.Event) {
	p, res := e.Burst.BPTCPayload()
	if res.Uncorrectable {
		return
	}
	h := pdu.DecodePIHeader(p)
	info := FromPIHeader(&h)
	if !info.Header {
		return
	}
	if r.voice && r.info.Header && info.MI == r.info.MI && info.Algorithm == r.info.Algorithm && info.KeyID == r.info.KeyID {
		return
	}
	r.voice, r.burst = false, 0
	r.stream.stop()
	if k := r.lookup(&info); k != nil {
		r.stream.start(k, info.MI)
		info.Decrypting = true
	}
	r.info = info
}

func (r *Receiver) voiceBurst(e *voice.Event, frames *[layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame) {
	if !r.voice {
		r.voice, r.burst = true, int(e.Lost)
	} else {
		r.burst += 1 + int(e.Lost)
	}
	if !r.stream.active() && !r.info.Header && r.info.Encrypted && r.basic != nil {
		r.stream.start(r.basic, 0)
		r.info.Decrypting = true
	}
	if idx, ok := e.Position.Index(); ok {
		for r.burst%6 != idx {
			r.burst++
		}
	}
	if frames == nil {
		return
	}
	*frames, _ = e.Burst.AMBEFrames()
	if !r.stream.active() {
		return
	}
	for i := range frames {
		r.stream.apply(r.burst*layer2.VoiceSlotsPerBurst+i, &frames[i], true)
	}
}
