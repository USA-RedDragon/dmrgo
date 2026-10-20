// Package dvsisim simulates a DVSI AMBE-3000R or AMBE-3003 vocoder chip in
// packet mode for tests. A Device is an io.ReadWriter that parses the
// packets written to it with its own parser, checks them against the
// manuals, and answers with stored frame and audio pairs. Faults can be
// injected: garbage bytes before a reply, lost replies, short reads and
// datagram reads.
package dvsisim

import (
	"strconv"
	"sync"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// Pairs returns n distinct pairs: frame i is the FEC-coded AMBE+2 frame of
// a test pattern and its audio a tone that depends on i.
func Pairs(n int) []Pair {
	p := make([]Pair, n)
	for i := range p {
		var f vocoder.AMBEFrame
		for j := range 6 {
			f[j] = byte(i*37 + j*91 + 5)
		}
		f[6] = byte(i) & 0x80
		p[i].Slot, _ = vocoder.EncodeAMBE72(f)
		for j := range p[i].PCM {
			p[i].PCM[j] = int16((j*(i+3)*131)%4001 - 2000)
		}
	}
	return p
}

// Pair is one 72-bit DMR AMBE+2 frame with FEC and the 160 samples that it
// decodes to and that encode to it.
type Pair struct {
	Slot [9]byte
	PCM  [160]int16
}

// Options configures a Device.
type Options struct {
	// Channels is 1 for an AMBE-3000R and 3 for an AMBE-3003. Zero means 1.
	Channels int
	// Datagram makes each Read return one whole reply, as a UDP socket to
	// an AMBEserver does.
	Datagram bool
	// Chunk caps the bytes of one Read in stream mode. Zero means no cap.
	Chunk int
	// NoParityPin starts the chip with parity fields off, as when its
	// PARITY_ENABLE pin is low.
	NoParityPin bool
}

// Product and Version are what the simulated chip reports.
const (
	Product3000 = "AMBE3000R"
	Product3003 = "AMBE3003"
	Version     = "V120.E100.XXXX.C106.G514.R009.B0010411.C0020208"
)

type timeoutError struct{}

func (timeoutError) Error() string { return "dvsisim: no data" }
func (timeoutError) Timeout() bool { return true }

// Counts counts the requests that a Device answered.
type Counts struct {
	Reset, Parity, Query, Rate, Decode, Encode int
}

// Device is a simulated chip. It is safe for concurrent use.
type Device struct {
	mu     sync.Mutex
	opt    Options
	pairs  []Pair
	parity bool
	rate   [3]bool
	init   [3]bool
	in     [4096]byte
	inN    int
	out    [8192]byte
	outN   int
	ends   [64]int
	endsN  int
	rbuf   [400]byte
	pbuf   [3 + 320]byte
	cbuf   [12]byte
	pcm    [160]int16
	resp   [128]byte

	// Garbage is written before the next reply and then cleared.
	Garbage []byte
	// Drop is the number of next replies to lose.
	Drop int
	// Reject is the status byte of the response to a RATEP field.
	Reject byte
	// Errs lists every problem found in the packets written.
	Errs []string
	// Counts counts the requests by kind.
	Counts Counts
}

// New returns a Device in its power-up state that answers with pairs.
func New(pairs []Pair, opt Options) *Device {
	if opt.Channels == 0 {
		opt.Channels = 1
	}
	d := &Device{opt: opt, pairs: pairs}
	d.powerUp()
	return d
}

func (d *Device) powerUp() {
	d.parity = !d.opt.NoParityPin
	d.rate, d.init = [3]bool{}, [3]bool{}
}

// Parity reports whether parity fields are on.
func (d *Device) Parity() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.parity
}

// Configured reports whether channel ch has the DMR rate and was
// initialized.
func (d *Device) Configured(ch int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rate[ch] && d.init[ch]
}

func (d *Device) fail(s string) {
	d.Errs = append(d.Errs, s)
}

// Write takes bytes from the host. It processes each complete packet and
// drops bytes before a start byte, as the chip does.
func (d *Device) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, b := range p {
		if d.inN == 0 && b != 0x61 {
			continue
		}
		if d.inN == len(d.in) {
			d.fail("input overflow")
			d.inN = 0
			continue
		}
		d.in[d.inN] = b
		d.inN++
		if d.inN >= 4 {
			n := int(d.in[1])<<8 | int(d.in[2])
			if 4+n > len(d.in) {
				d.fail("length " + strconv.Itoa(n))
				d.inN = 0
				continue
			}
			if d.inN == 4+n {
				d.packet(d.in[:d.inN])
				d.inN = 0
			}
		}
	}
	return len(p), nil
}

// Read returns reply bytes, or 0 and an error whose Timeout method returns
// true when there are none.
func (d *Device) Read(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.outN == 0 {
		return 0, timeoutError{}
	}
	if d.opt.Datagram {
		n := d.ends[0]
		if n > len(p) {
			d.fail("datagram read into " + strconv.Itoa(len(p)) + " bytes")
		}
		k := copy(p, d.out[:n])
		d.pop(n)
		return k, nil
	}
	n := min(d.outN, len(p))
	if d.opt.Chunk > 0 {
		n = min(n, d.opt.Chunk)
	}
	copy(p, d.out[:n])
	d.pop(n)
	return n, nil
}

func (d *Device) pop(n int) {
	d.outN = copy(d.out[:], d.out[n:d.outN])
	k := 0
	for i := range d.endsN {
		if e := d.ends[i] - n; e > 0 {
			d.ends[k] = e
			k++
		}
	}
	d.endsN = k
}

func (d *Device) emit(b []byte) {
	if d.outN+len(b) > len(d.out) || d.endsN == len(d.ends) {
		d.fail("output overflow")
		return
	}
	d.outN += copy(d.out[d.outN:], b)
	d.ends[d.endsN] = d.outN
	d.endsN++
}

func (d *Device) reply(t byte, fields []byte) {
	if d.Drop > 0 {
		d.Drop--
		return
	}
	if len(d.Garbage) > 0 {
		d.emit(d.Garbage)
		d.Garbage = nil
	}
	n := len(fields)
	if d.parity {
		n += 2
	}
	b := append(d.rbuf[:0], 0x61, byte(n>>8), byte(n), t)
	b = append(b, fields...)
	if d.parity {
		b = append(b, 0x2F)
		var x byte
		for _, c := range b[1:] {
			x ^= c
		}
		b = append(b, x)
	}
	d.emit(b)
}

func (d *Device) packet(b []byte) {
	n := len(b) - 4
	fields := b[4:]
	if d.parity {
		var x byte
		for _, c := range b[1 : len(b)-1] {
			x ^= c
		}
		if n < 2 || fields[n-2] != 0x2F || fields[n-1] != x {
			d.fail("parity")
			return
		}
		fields = fields[:n-2]
	} else if n >= 2 && fields[n-2] == 0x2F {
		var x byte
		for _, c := range b[1 : len(b)-1] {
			x ^= c
		}
		if fields[n-1] == x {
			fields = fields[:n-2]
		}
	}
	switch b[3] {
	case 0x00:
		d.control(fields)
	case 0x01:
		d.channel(fields)
	case 0x02:
		d.speech(fields)
	default:
		d.fail("packet type " + strconv.Itoa(int(b[3])))
	}
}

func (d *Device) control(f []byte) {
	r := d.resp[:0]
	ch := 0
	for len(f) > 0 {
		id := f[0]
		f = f[1:]
		switch {
		case id >= 0x40 && id <= 0x42:
			ch = int(id - 0x40)
			if ch >= d.opt.Channels {
				d.fail("channel " + strconv.Itoa(ch))
				return
			}
			r = append(r, id, 0)
		case id == 0x33:
			d.Counts.Reset++
			if len(f) != 0 {
				d.fail("fields after reset")
			}
			d.powerUp()
			d.reply(0x00, []byte{0x39})
			return
		case id == 0x3F:
			if len(f) < 1 || f[0] > 1 {
				d.fail("parity mode")
				return
			}
			d.Counts.Parity++
			d.parity = f[0] == 1
			r = append(r, id, 0)
			f = f[1:]
		case id == 0x30:
			d.Counts.Query++
			prod := Product3000
			if d.opt.Channels == 3 {
				prod = Product3003
			}
			r = append(append(r, id), prod...)
			r = append(r, 0)
		case id == 0x31:
			r = append(append(r, id), Version...)
			r = append(r, 0)
		case id == 0x0A:
			if len(f) < 12 {
				d.fail("short RATEP")
				return
			}
			d.Counts.Rate++
			ok := [12]byte(f[:12]) == [12]byte{0x04, 0x31, 0x07, 0x54, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00, 0x6F, 0x48}
			if !ok {
				d.fail("RATEP is not the DMR rate")
			}
			d.rate[ch] = ok
			r = append(r, id, d.Reject)
			f = f[12:]
		case id == 0x09:
			if len(f) < 1 {
				d.fail("short RATET")
				return
			}
			d.rate[ch] = f[0] == 33
			r = append(r, id, 0)
			f = f[1:]
		case id == 0x0B:
			if len(f) < 1 || f[0]&3 != 3 {
				d.fail("INIT does not set up the encoder and decoder")
				return
			}
			d.init[ch] = true
			r = append(r, id, 0)
			f = f[1:]
		case id == 0x2F:
			if len(f) < 1 {
				d.fail("short parity field")
				return
			}
			f = f[1:]
		default:
			d.fail("control field " + strconv.Itoa(int(id)))
			return
		}
	}
	d.reply(0x00, r)
}

func (d *Device) dataChannel(f []byte) (int, []byte, bool) {
	ch := 0
	if len(f) > 0 && f[0] >= 0x40 && f[0] <= 0x42 {
		ch = int(f[0] - 0x40)
		f = f[1:]
	}
	if ch >= d.opt.Channels {
		d.fail("channel " + strconv.Itoa(ch))
		return 0, nil, false
	}
	if !d.rate[ch] || !d.init[ch] {
		d.fail("channel " + strconv.Itoa(ch) + " used before it was configured")
		return 0, nil, false
	}
	return ch, f, true
}

func (d *Device) channel(f []byte) {
	ch, f, ok := d.dataChannel(f)
	if !ok {
		return
	}
	if len(f) != 11 || f[0] != 0x01 || f[1] != 72 {
		d.fail("channel packet is not one CHAND field of 72 bits")
		return
	}
	d.Counts.Decode++
	slot := [9]byte(f[2:11])
	pcm := &d.pcm
	*pcm = [160]int16{}
	found := false
	for i := range d.pairs {
		if d.pairs[i].Slot == slot {
			pcm, found = &d.pairs[i].PCM, true
			break
		}
	}
	if !found {
		d.fail("unknown frame")
	}
	r := d.pbuf[:0]
	if ch > 0 {
		r = append(r, 0x40+byte(ch))
	}
	r = append(r, 0x00, 160)
	for _, s := range pcm {
		r = append(r, byte(uint16(s)>>8), byte(s))
	}
	d.reply(0x02, r)
}

func (d *Device) speech(f []byte) {
	ch, f, ok := d.dataChannel(f)
	if !ok {
		return
	}
	if len(f) != 2+320 || f[0] != 0x00 || f[1] != 160 {
		d.fail("speech packet is not one SPEECHD field of 160 samples")
		return
	}
	d.Counts.Encode++
	pcm := &d.pcm
	for i := range pcm {
		pcm[i] = int16(uint16(f[2+2*i])<<8 | uint16(f[3+2*i]))
	}
	slot := [9]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x00, 0x2A, 0x6B}
	found := false
	for i := range d.pairs {
		if d.pairs[i].PCM == *pcm {
			slot, found = d.pairs[i].Slot, true
			break
		}
	}
	if !found {
		d.fail("unknown audio")
	}
	r := d.cbuf[:0]
	if ch > 0 {
		r = append(r, 0x40+byte(ch))
	}
	r = append(r, 0x01, 72)
	r = append(r, slot[:]...)
	d.reply(0x01, r)
}
