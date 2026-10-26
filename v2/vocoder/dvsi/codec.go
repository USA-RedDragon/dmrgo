package dvsi

import (
	"bytes"
	"errors"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// Port carries packets to and from a chip. Any io.ReadWriter is a Port,
// such as an *os.File of a serial device or a net.Conn to an AMBEserver.
// Read returns what has arrived, and should wait for data up to a time
// limit of its own; a Read that returns 0 bytes and no error is retried
// until Config.Timeout. When the Port also has a SetReadDeadline(time.Time)
// error method, as *os.File and net.Conn have, and it accepts a zero
// deadline in New, Codec sets the deadline of each reply with it. A datagram Port must return one whole datagram per
// Read into a buffer of MaxPacketLen bytes.
type Port interface {
	// Read reads bytes from the chip into p.
	Read(p []byte) (int, error)
	// Write writes the bytes of p to the chip.
	Write(p []byte) (int, error)
}

// DefaultTimeout is the time a Codec waits for a reply when Config.Timeout
// is zero.
const DefaultTimeout = 500 * time.Millisecond

// Config selects how New sets up the chip.
type Config struct {
	// Channel is the vocoder channel of the chip, 0 to MaxChannel. The
	// AMBE-3000R has only channel 0.
	Channel int
	// Timeout is the longest wait for each reply. Zero selects
	// DefaultTimeout.
	Timeout time.Duration
	// NoReset leaves out the reset of the chip, for a second Codec on
	// another channel of an AMBE-3003 that is already in use.
	NoReset bool
	// NoParity turns the parity fields off. By default every packet carries
	// a parity field and the chip drops packets with a wrong parity byte.
	NoParity bool
}

type deadliner interface {
	SetReadDeadline(t time.Time) error
}

type timeouter interface {
	Timeout() bool
}

// Codec is a vocoder.Codec that runs on a DVSI chip at the DMR AMBE+2
// rate. Each Decode, DecodeLost and Encode sends one packet and waits for
// the reply. After a request that got no reply, the next one first sends a
// product ID query and drops the replies that come before its response,
// so that a late reply does not answer a later request. A Codec is not
// safe for concurrent use, and Codecs that share one Port must not be used
// at the same time. The zero Codec is closed.
type Codec struct {
	port     Port
	dl       deadliner
	timeout  time.Duration
	channel  int
	parity   bool
	closed   bool
	stale    bool
	lost     bool
	query    []byte
	clear    []byte
	product  string
	version  string
	rxN      int
	rx       [2 * MaxPacketLen]byte
	tx       []byte
	silence  [FrameBits / 8]byte
	resetPad [350]byte
}

var _ vocoder.Codec = (*Codec)(nil)

// New sets up the chip behind port for the DMR AMBE+2 rate and returns a
// Codec for it. Unless cfg.NoReset, it first sends 350 zero bytes, which end
// any packet the chip is in the middle of, and a reset packet, and waits for
// PKT_READY, as xlxd ambed does. Then it sets the parity mode, reads the
// product name and version, sets the rate of cfg.Channel with AppendRateDMR,
// and with a PKT_SPCHFMT field of 0x0001 for cfg.Channel has the chip put
// the decoder status DCMODE_OUT in a CMODE field of each speech packet
// (AMBE-3000R manual Tables 63 and 64). Last, a PKT_DCMODE field of 0x0000
// clears the DCMODE_IN flags of cfg.Channel, which the chip keeps until they
// are changed, so that a LOST_FRAME left set by an earlier user of the
// channel does not make the chip repeat every frame (Tables 14 and 36). A
// nil cfg means the zero Config. It returns dmrgo.ErrNilArgument for a nil
// port, ErrChannel for a channel outside 0 to MaxChannel, ErrTimeout when
// the chip does not reply, ErrSpeechFormat when it rejects the PKT_SPCHFMT
// field, ErrStatus when it rejects another field, with a text that names
// PKT_DCMODE for that field, and the errors of port. It allocates the Codec,
// the product and version strings, the query packet that resynchronizes it
// after a timeout, and the PKT_DCMODE packet.
func New(port Port, cfg *Config) (*Codec, error) {
	var conf Config
	if cfg != nil {
		conf = *cfg
	}
	cfg = &conf
	if port == nil {
		return nil, dmrgo.ErrNilArgument
	}
	if cfg.Channel < 0 || cfg.Channel > MaxChannel {
		return nil, ErrChannel
	}
	c := &Codec{port: port, timeout: cfg.Timeout, channel: cfg.Channel, parity: true, tx: make([]byte, 0, MaxPacketLen)}
	if c.timeout <= 0 {
		c.timeout = DefaultTimeout
	}
	if dl, ok := port.(deadliner); ok && dl.SetReadDeadline(time.Time{}) == nil {
		c.dl = dl
	}
	c.silence = vocoder.AMBESilenceSlot()
	if !cfg.NoReset {
		if err := c.reset(); err != nil {
			return nil, err
		}
	}
	c.parity = !cfg.NoParity
	_, n, err := c.control(AppendParityMode(c.tx[:0], c.parity, true))
	if err != nil {
		return nil, err
	}
	c.consume(n)
	c.query = AppendQuery(nil, c.parity)
	c.clear = appendDCMode(nil, c.channel, 0, c.parity)
	p, n, err := c.control(AppendQuery(c.tx[:0], c.parity))
	if err != nil {
		return nil, err
	}
	prod, _ := p.Response(FieldProdID)
	ver, _ := p.Response(FieldVerString)
	prod, ver = prod[:min(len(prod), MaxProductLen)], ver[:min(len(ver), MaxVersionLen)]
	c.product, c.version = string(prod), string(ver)
	c.consume(n)
	pkt, _ := AppendRateDMR(c.tx[:0], c.channel, c.parity)
	if _, n, err = c.control(pkt); err != nil {
		return nil, err
	}
	c.consume(n)
	_, n, err = c.control(appendSpeechFormat(c.tx[:0], c.channel, c.parity))
	if errors.Is(err, ErrStatus) {
		return nil, ErrSpeechFormat
	}
	if err != nil {
		return nil, err
	}
	c.consume(n)
	if err := c.clearDCMode(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Codec) clearDCMode() error {
	_, n, err := c.control(c.clear)
	if errors.Is(err, ErrStatus) {
		return errDCMode
	}
	if err != nil {
		return err
	}
	c.consume(n)
	c.lost = false
	return nil
}

func (c *Codec) reset() error {
	if err := c.write(c.resetPad[:]); err != nil {
		return err
	}
	if err := c.write(AppendReset(c.tx[:0], true)); err != nil {
		return err
	}
	start := time.Now()
	for {
		p, n, err := c.next(start, false, TypeControl)
		if err != nil {
			return err
		}
		ok := p.Type == TypeControl && len(p.Fields) == 1 && p.Fields[0] == byte(FieldReady)
		if !ok && len(p.Fields) == 3 {
			q, _, st := parse(c.rx[:n], true)
			ok = st == parseOK && q.Type == TypeControl && len(q.Fields) == 1 && q.Fields[0] == byte(FieldReady)
		}
		if ok {
			c.consume(n)
			return nil
		}
		c.consume(1)
	}
}

func (c *Codec) control(pkt []byte) (Packet, int, error) {
	first := pkt[HeaderLen]
	if err := c.write(pkt); err != nil {
		return Packet{}, 0, err
	}
	start := time.Now()
	for {
		p, n, err := c.next(start, c.parity, TypeControl)
		if err != nil {
			return Packet{}, 0, err
		}
		if p.Type != TypeControl || len(p.Fields) == 0 || p.Fields[0] != first {
			c.consume(1)
			continue
		}
		if p.failed() {
			c.consume(n)
			return Packet{}, 0, ErrStatus
		}
		return p, n, nil
	}
}

func (c *Codec) write(b []byte) error {
	for len(b) > 0 {
		n, err := c.port.Write(b)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(b) {
			return ErrPort
		}
		b = b[n:]
	}
	return nil
}

func (c *Codec) consume(n int) {
	c.rxN = copy(c.rx[:], c.rx[n:c.rxN])
}

func (c *Codec) next(start time.Time, parity bool, want PacketType) (Packet, int, error) {
	for {
		if i := bytes.IndexByte(c.rx[:c.rxN], StartByte); i < 0 {
			c.rxN = 0
		} else if i > 0 {
			c.consume(i)
		}
		if c.rxN > 0 {
			p, n, st := parse(c.rx[:c.rxN], parity)
			if st == parseOK {
				return p, n, nil
			}
			if st != parseIncomplete {
				c.consume(1)
				continue
			}
			if i := c.later(parity, want); i > 0 {
				c.consume(i)
				continue
			}
		}
		if err := c.fill(start); err != nil {
			return Packet{}, 0, err
		}
	}
}

func (c *Codec) later(parity bool, want PacketType) int {
	for i := 1; i < c.rxN; i++ {
		if c.rx[i] != StartByte {
			continue
		}
		if p, _, st := parse(c.rx[i:c.rxN], parity); st == parseOK && p.Type == want {
			return i
		}
	}
	return 0
}

func (c *Codec) fill(start time.Time) error {
	for {
		if time.Since(start) >= c.timeout {
			return ErrTimeout
		}
		if c.dl != nil {
			if err := c.dl.SetReadDeadline(start.Add(c.timeout)); err != nil {
				return err
			}
		}
		n, err := c.port.Read(c.rx[c.rxN:])
		if n < 0 || n > len(c.rx)-c.rxN {
			return ErrPort
		}
		if n > 0 {
			c.rxN += n
			return nil
		}
		if err != nil {
			if t, ok := err.(timeouter); !ok || !t.Timeout() {
				return err
			}
		}
	}
}

func (c *Codec) resync() error {
	if !c.stale {
		return nil
	}
	_, n, err := c.control(c.query)
	if err != nil {
		return err
	}
	c.consume(n)
	c.stale = false
	return nil
}

func (c *Codec) send(pkt []byte) (time.Time, error) {
	if err := c.write(pkt); err != nil {
		return time.Time{}, err
	}
	return time.Now(), nil
}

// MaxProductLen and MaxVersionLen are the longest product name and version
// string that a Codec keeps (AMBE-3000R manual Tables 67 and 69).
const (
	MaxProductLen = 16
	MaxVersionLen = 48
)

// Product returns the product name that the chip reported in New, such as
// "AMBE3000R". It returns at most MaxProductLen bytes.
func (c *Codec) Product() string {
	if c == nil {
		return ""
	}
	return c.product
}

// Version returns the version string that the chip reported in New. It
// returns at most MaxVersionLen bytes.
func (c *Codec) Version() string {
	if c == nil {
		return ""
	}
	return c.version
}

// FrameSamples returns FrameSamples.
func (c *Codec) FrameSamples() int {
	if c == nil {
		return 0
	}
	return FrameSamples
}

// FID returns enums.FIDStandardized, the Feature set ID of calls with the
// ETSI standard vocoder, AMBE+2.
func (c *Codec) FID() enums.FID {
	if c == nil {
		return 0
	}
	return enums.FIDStandardized
}

// Silence returns vocoder.AMBESilenceSlot.
func (c *Codec) Silence() [9]byte {
	if c == nil {
		return [9]byte{}
	}
	return c.silence
}

// Encode has the chip encode exactly FrameSamples samples into one 72-bit
// frame with FEC. It returns ErrSamples for another number of samples,
// ErrClosed after Close, ErrTimeout when the chip does not reply, and the
// errors of the Port. It does not allocate.
func (c *Codec) Encode(pcm []int16) ([9]byte, error) {
	if c == nil {
		return [9]byte{}, dmrgo.ErrNilReceiver
	}

	var slot [9]byte
	if c.closed || c.port == nil {
		return slot, ErrClosed
	}
	if len(pcm) != FrameSamples {
		return slot, ErrSamples
	}
	if err := c.resync(); err != nil {
		return slot, err
	}
	n := putSpeech(c.tx[:cap(c.tx)], c.channel, pcm, c.parity)
	start, err := c.send(c.tx[:n])
	if err != nil {
		return slot, err
	}
	for {
		p, n, err := c.next(start, c.parity, TypeChannel)
		if err != nil {
			c.stale = true
			return slot, err
		}
		ch, body, _, err := p.walk(TypeChannel, fieldChannelData)
		if err != nil || ch != c.channel || body[0] != FrameBits {
			c.consume(1)
			continue
		}
		copy(slot[:], body[1:])
		c.consume(n)
		return slot, nil
	}
}

// Decode has the chip decode one 72-bit frame with FEC, appends the
// FrameSamples samples to dst and returns the extended slice and a report
// from the DCMODE_OUT flags of the chip (AMBE-3000R manual Table 15):
// Output is OutputTone for TONE_FRAME, else OutputSilence when
// VOICE_ACTIVE is 0, else OutputRepeat when DATA_INVALID is 1, else
// OutputVoice; Frame is FrameErased when DATA_INVALID is 1, else
// FrameGood. The chip reports no bit counts, so CountsKnown is false. A
// reply without a CMODE field, which a chip that accepted PKT_SPCHFMT in New
// does not send, reports FrameGood and OutputVoice. After DecodeLost the
// request also clears LOST_FRAME. It returns ErrClosed after Close,
// ErrTimeout when the chip does not reply, and the errors of the Port, with
// dst unchanged and a zero report. It allocates only when dst has no room
// for FrameSamples more samples.
func (c *Codec) Decode(dst []int16, slot [9]byte) ([]int16, vocoder.FrameReport, error) {
	if c == nil {
		return dst, vocoder.FrameReport{}, dmrgo.ErrNilReceiver
	}

	if c.closed || c.port == nil {
		return dst, vocoder.FrameReport{}, ErrClosed
	}
	if err := c.resync(); err != nil {
		return dst, vocoder.FrameReport{}, err
	}
	var pkt []byte
	if c.lost {
		pkt = appendCMode(c.tx[:0], c.channel, &slot, 0, c.parity)
	} else {
		pkt, _ = AppendChannel(c.tx[:0], c.channel, slot[:], FrameBits, c.parity)
	}
	out, mode, err := c.decode(dst, pkt)
	if err != nil {
		return dst, vocoder.FrameReport{}, err
	}
	c.lost = false
	return out, report(mode, false), nil
}

// DecodeLost has the chip fill a frame that never arrived: it sends a
// channel packet with no channel bits and a CMODE field that sets
// LOST_FRAME, and the chip repeats the frame before, or after more than 2
// repeats outputs comfort noise (AMBE-3000R manual Tables 14 and 105). It
// appends the FrameSamples samples to dst and returns the extended slice
// and a report with Frame FrameLost and Output from the DCMODE_OUT flags as
// for Decode, OutputRepeat for a reply without a CMODE field. It returns
// the errors of Decode, with dst unchanged and a zero report. It allocates
// only when dst has no room for FrameSamples more samples.
func (c *Codec) DecodeLost(dst []int16) ([]int16, vocoder.FrameReport, error) {
	if c == nil {
		return dst, vocoder.FrameReport{}, dmrgo.ErrNilReceiver
	}

	if c.closed || c.port == nil {
		return dst, vocoder.FrameReport{}, ErrClosed
	}
	if err := c.resync(); err != nil {
		return dst, vocoder.FrameReport{}, err
	}
	c.lost = true
	out, mode, err := c.decode(dst, appendCMode(c.tx[:0], c.channel, nil, dcmodeLostFrame, c.parity))
	if err != nil {
		return dst, vocoder.FrameReport{}, err
	}
	return out, report(mode, true), nil
}

func (c *Codec) decode(dst []int16, pkt []byte) ([]int16, cmode, error) {
	start, err := c.send(pkt)
	if err != nil {
		return dst, cmode{}, err
	}
	for {
		p, n, err := c.next(start, c.parity, TypeSpeech)
		if err != nil {
			c.stale = true
			return dst, cmode{}, err
		}
		out, ch, err := p.AppendSamples(dst)
		if err != nil || ch != c.channel || len(out)-len(dst) != FrameSamples {
			c.consume(1)
			continue
		}
		_, _, mode, _ := p.walk(TypeSpeech, fieldSpeechData)
		c.consume(n)
		return out, mode, nil
	}
}

func report(mode cmode, lost bool) vocoder.FrameReport {
	var r vocoder.FrameReport
	if lost {
		r.Frame, r.Output = vocoder.FrameLost, vocoder.OutputRepeat
	}
	if !mode.ok {
		return r
	}
	invalid := mode.flags&dcmodeDataInvalid != 0
	switch {
	case mode.flags&dcmodeToneFrame != 0:
		r.Output = vocoder.OutputTone
	case mode.flags&dcmodeVoiceActive == 0:
		r.Output = vocoder.OutputSilence
	case invalid:
		r.Output = vocoder.OutputRepeat
	default:
		r.Output = vocoder.OutputVoice
	}
	if invalid && !lost {
		r.Frame = vocoder.FrameErased
	}
	return r
}

// Reset does nothing.
func (c *Codec) Reset() {}

// Close marks the Codec closed; later Encode, Decode and DecodeLost calls
// return ErrClosed. When the last decode request was DecodeLost, Close first
// clears LOST_FRAME with a PKT_DCMODE field of 0x0000, so that the chip does
// not keep repeating frames for the next user of the channel, and returns
// ErrTimeout when the chip does not reply, ErrStatus when it rejects the
// field, and the errors of the Port; the Codec is closed in any case. It
// does not close the Port. Close is idempotent; a later Close returns nil.
// It does not allocate.
func (c *Codec) Close() error {
	if c == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.closed {
		return nil
	}
	c.closed = true
	if !c.lost || c.port == nil {
		return nil
	}
	if err := c.resync(); err != nil {
		return err
	}
	return c.clearDCMode()
}
