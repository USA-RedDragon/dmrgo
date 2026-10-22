package dvsi

import (
	"bytes"
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
// rate. Each Decode and Encode sends one packet and waits for the reply. A
// Codec is not safe for concurrent use, and Codecs that share one Port
// must not be used at the same time. The zero Codec is closed.
type Codec struct {
	port     Port
	dl       deadliner
	timeout  time.Duration
	channel  int
	parity   bool
	closed   bool
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
// Codec for it. Unless cfg.NoReset, it first sends 350 zero bytes, which
// end any packet the chip is in the middle of, and a reset packet, and
// waits for PKT_READY, as xlxd ambed does. Then it sets the parity mode,
// reads the product name and version, and sets the rate of cfg.Channel
// with AppendRateDMR. A nil cfg means the zero Config. It returns
// dmrgo.ErrNilArgument for a nil port, ErrChannel for a channel outside 0
// to MaxChannel, ErrTimeout when the chip does not reply, ErrStatus when it rejects
// a field, and the errors of port. It allocates the Codec and the product
// and version strings.
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
	return c, nil
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
	n := putSpeech(c.tx[:cap(c.tx)], c.channel, pcm, c.parity)
	start, err := c.send(c.tx[:n])
	if err != nil {
		return slot, err
	}
	for {
		p, n, err := c.next(start, c.parity, TypeChannel)
		if err != nil {
			return slot, err
		}
		ch, body, err := p.walk(TypeChannel, fieldChannelData)
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
// FrameSamples samples to dst and returns the extended slice. It returns
// ErrClosed after Close, ErrTimeout when the chip does not reply, and the
// errors of the Port, with dst unchanged. It allocates only when dst has no
// room for FrameSamples more samples.
func (c *Codec) Decode(dst []int16, slot [9]byte) ([]int16, error) {
	if c == nil {
		return dst, dmrgo.ErrNilReceiver
	}

	if c.closed || c.port == nil {
		return dst, ErrClosed
	}
	pkt, err := AppendChannel(c.tx[:0], c.channel, slot[:], FrameBits, c.parity)
	if err != nil {
		return dst, err
	}
	start, err := c.send(pkt)
	if err != nil {
		return dst, err
	}
	for {
		p, n, err := c.next(start, c.parity, TypeSpeech)
		if err != nil {
			return dst, err
		}
		out, ch, err := p.AppendSamples(dst)
		if err != nil || ch != c.channel || len(out)-len(dst) != FrameSamples {
			c.consume(1)
			continue
		}
		c.consume(n)
		return out, nil
	}
}

// Reset does nothing: the Codec keeps no state between frames.
func (c *Codec) Reset() {}

// Close marks the Codec closed; later Encode and Decode calls return
// ErrClosed. It does not close the Port. Close is idempotent and returns
// nil.
func (c *Codec) Close() error {
	if c == nil {
		return dmrgo.ErrNilReceiver
	}

	c.closed = true
	return nil
}
