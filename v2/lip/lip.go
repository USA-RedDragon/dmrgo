package lip

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// Extension is the PDU type extension of a LIP PDU of PDU type 1. Values
// that have no constant are reserved (ETSI TS 100 392-18-1 clause 6.3.62).
type Extension uint8

// The Extension values.
const (
	ExtensionImmediateReport   Extension = 0b0001 // immediate location report request
	ExtensionLongReport        Extension = 0b0011 // long location report
	ExtensionReportAck         Extension = 0b0100 // location report acknowledgement
	ExtensionBasicParameters   Extension = 0b0101 // basic location parameters request or response
	ExtensionAddModifyTrigger  Extension = 0b0110 // add/modify trigger request or response
	ExtensionRemoveTrigger     Extension = 0b0111 // remove trigger request or response
	ExtensionReportTrigger     Extension = 0b1000 // report trigger request or response
	ExtensionReportBasicParams Extension = 0b1001 // report basic location parameters request or response
	ExtensionEnableDisable     Extension = 0b1010 // location reporting enable/disable request or response
	ExtensionTemporaryControl  Extension = 0b1011 // location reporting temporary control request or response
	ExtensionBacklog           Extension = 0b1100 // backlog request or response
)

// String returns the name of the Extension. It allocates only for a
// reserved value.
func (e Extension) String() string {
	switch e {
	case ExtensionImmediateReport:
		return "Immediate location report request"
	case ExtensionLongReport:
		return "Long location report"
	case ExtensionReportAck:
		return "Location report acknowledgement"
	case ExtensionBasicParameters:
		return "Basic location parameters"
	case ExtensionAddModifyTrigger:
		return "Add/modify trigger"
	case ExtensionRemoveTrigger:
		return "Remove trigger"
	case ExtensionReportTrigger:
		return "Report trigger"
	case ExtensionReportBasicParams:
		return "Report basic location parameters"
	case ExtensionEnableDisable:
		return "Location reporting enable/disable"
	case ExtensionTemporaryControl:
		return "Location reporting temporary control"
	case ExtensionBacklog:
		return "Backlog"
	}
	return "Extension(" + text.Int(int64(e)) + ")"
}

// HasResponseBit reports whether a PDU with the extension e carries the
// request/response bit after the extension: the immediate location report
// request and the request and response PDUs from
// ExtensionBasicParameters to ExtensionBacklog. (ETSI TS 100 392-18-1
// clause 6.2)
func (e Extension) HasResponseBit() bool {
	return e == ExtensionImmediateReport || e >= ExtensionBasicParameters && e <= ExtensionBacklog
}

// Header is the start of a LIP PDU: the PDU type and, for PDU type 1, the
// PDU type extension and the request/response bit.
type Header struct {
	// Short is true for PDU type 0, the short location report. The other
	// fields are then zero.
	Short bool
	// Extension is the PDU type extension of PDU type 1.
	Extension Extension
	// Response is the request/response bit, true for a response, for an
	// Extension with HasResponseBit.
	Response bool
}

// DecodeHeader decodes the start of the LIP PDU pdu. It returns ErrShort
// for a PDU without the bits of the header, and ErrPDUType for the
// reserved PDU types 2 and 3. (ETSI TS 100 392-18-1 clauses 6.3.61 and
// 6.3.62)
func DecodeHeader(pdu []byte) (Header, error) {
	r := reader{b: pdu}
	switch r.u(2) {
	case 0:
		return Header{Short: true}, r.err
	case 1:
	default:
		return Header{}, ErrPDUType
	}
	h := Header{Extension: Extension(r.u8(4))}
	if h.Extension.HasResponseBit() {
		h.Response = r.u(1) == 1
	}
	if r.err != nil {
		return Header{}, r.err
	}
	return h, nil
}

func (r *reader) start(ext Extension, response int) {
	if r.u(2) != 1 || Extension(r.u8(4)) != ext || (response >= 0 && int(r.u8(1)) != response) {
		r.fail(ErrPDUType)
	}
}

func (r *reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

type reader struct {
	b   []byte
	at  int
	err error
}

func (r *reader) u(n int) uint64 {
	if r.err != nil {
		return 0
	}
	if r.at+n > len(r.b)*8 {
		r.err = ErrShort
		return 0
	}
	v := bits.Field64(r.b, r.at, n)
	r.at += n
	return v
}

func (r *reader) flag() bool {
	return r.u(1) == 1
}

func (r *reader) u8(n int) uint8 {
	return uint8(r.u(n) & 0xFF)
}

func (r *reader) u16(n int) uint16 {
	return uint16(r.u(n) & 0xFFFF)
}

func (r *reader) coordinate(n int, degrees float64) float64 {
	v := r.u(n)
	f := float64(v)
	if v >= 1<<(n-1) {
		f -= float64(uint64(1) << n)
	}
	return f * degrees / float64(uint64(1)<<n)
}

const maxPDUOctets = 32

type writer struct {
	b   [maxPDUOctets]byte
	at  int
	err error
}

func (w *writer) put(n int, v uint64, e error) {
	if w.err != nil {
		return
	}
	if n < 64 && v>>n != 0 {
		w.fail(e)
		return
	}
	bits.SetField64(w.b[:], w.at, n, v)
	w.at += n
}

func (w *writer) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

func (w *writer) flag(v bool) {
	var b uint64
	if v {
		b = 1
	}
	w.put(1, b, nil)
}

func (w *writer) start(ext Extension, response int) {
	w.put(2, 1, nil)
	w.put(4, uint64(ext), nil)
	if response >= 0 {
		w.put(1, uint64(response), nil)
	}
}

func (w *writer) finish(dst []byte) ([]byte, int, error) {
	if w.err != nil {
		return dst, 0, w.err
	}
	return append(dst, w.b[:(w.at+7)/8]...), w.at, nil
}
