package lip

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// ReportType is the kind of location report that a request specifies (ETSI
// TS 100 392-18-1 clause 6.3.65).
type ReportType uint8

// The ReportType values.
const (
	ReportLongNoTime   ReportType = 0 // a long location report without time
	ReportLongElapsed  ReportType = 1 // a long location report with TimeElapsed
	ReportLongPosition ReportType = 2 // a long location report with TimePosition
	ReportShort        ReportType = 3 // a short location report
)

// String returns the name of the ReportType. It allocates only for a value
// over 3.
func (t ReportType) String() string {
	switch t {
	case ReportLongNoTime:
		return "Long location report with no time"
	case ReportLongElapsed:
		return "Long location report with time elapsed"
	case ReportLongPosition:
		return "Long location report with time of position"
	case ReportShort:
		return "Short location report"
	}
	return "ReportType(" + text.Int(int64(t)) + ")"
}

// ReportAck is the LOCATION REPORT ACKNOWLEDGEMENT PDU, the answer to a
// LongReport with AckRequested. Its type 5 elements follow from bit
// Elements. (ETSI TS 100 392-18-1 clause 6.2.3)
type ReportAck struct {
	// Reserved is reserved for result reasons, 0.
	Reserved uint8
	// Elements is the bit offset of the first type 5 element.
	// DecodeReportAck sets it; Encode ignores it.
	Elements int
}

// DecodeReportAck decodes the LOCATION REPORT ACKNOWLEDGEMENT PDU pdu. It
// returns ErrPDUType for another PDU and ErrShort for a PDU that ends
// early. (ETSI TS 100 392-18-1 clause 6.2.3)
func DecodeReportAck(pdu []byte) (ReportAck, error) {
	r := reader{b: pdu}
	r.start(ExtensionReportAck, -1)
	a := ReportAck{Reserved: r.u8(8)}
	if r.err != nil {
		return ReportAck{}, r.err
	}
	a.Elements = r.at
	return a, nil
}

// Encode appends the PDU of a to dst, with 0 bits after its last bit, and
// returns dst and the number of bits of the PDU. It returns
// dmrgo.ErrNilReceiver for a nil a, and allocates only to grow dst. (ETSI
// TS 100 392-18-1 clause 6.2.3)
func (a *ReportAck) Encode(dst []byte) ([]byte, int, error) {
	if a == nil {
		return dst, 0, dmrgo.ErrNilReceiver
	}
	var w writer
	w.start(ExtensionReportAck, -1)
	w.put(8, uint64(a.Reserved), nil)
	return w.finish(dst)
}

// ImmediateRequest is the IMMEDIATE LOCATION REPORT REQUEST PDU. Its type
// 5 elements, such as the location information destination and the
// maximum response time, follow from bit Elements. (ETSI TS 100 392-18-1
// clause 6.2.16)
type ImmediateRequest struct {
	// ReportType is the kind of report that the request specifies.
	ReportType ReportType
	// Elements is the bit offset of the first type 5 element.
	// DecodeImmediateRequest sets it; Encode ignores it.
	Elements int
}

// DecodeImmediateRequest decodes the IMMEDIATE LOCATION REPORT REQUEST PDU
// pdu. It returns ErrPDUType for another PDU or a response, and ErrShort
// for a PDU that ends early. (ETSI TS 100 392-18-1 clause 6.2.16)
func DecodeImmediateRequest(pdu []byte) (ImmediateRequest, error) {
	r := reader{b: pdu}
	r.start(ExtensionImmediateReport, 0)
	q := ImmediateRequest{ReportType: ReportType(r.u8(2))}
	if r.err != nil {
		return ImmediateRequest{}, r.err
	}
	q.Elements = r.at
	return q, nil
}

// Encode appends the PDU of q to dst, with 0 bits after its last bit, and
// returns dst and the number of bits of the PDU. It returns a
// *dmrgo.FieldRangeError for a ReportType over 3 and dmrgo.ErrNilReceiver
// for a nil q, and allocates only to grow dst. (ETSI TS 100 392-18-1
// clause 6.2.16)
func (q *ImmediateRequest) Encode(dst []byte) ([]byte, int, error) {
	if q == nil {
		return dst, 0, dmrgo.ErrNilReceiver
	}
	var w writer
	w.start(ExtensionImmediateReport, 0)
	w.put(2, uint64(q.ReportType), errReportType)
	return w.finish(dst)
}

// EnableDisable is the LOCATION REPORTING ENABLE/DISABLE REQUEST or
// RESPONSE PDU, which every MS that supports LIP handles. Its type 5
// elements, such as the terminal or location identification, follow from
// bit Elements. (ETSI TS 100 392-18-1 clauses 6.2.14, 6.2.15 and 7)
type EnableDisable struct {
	// Response is true for the response PDU, false for the request.
	Response bool
	// AckRequested requests an acknowledgement. Only the request codes
	// it.
	AckRequested bool
	// Flags holds the location reporting enable flags, flag 1 in the most
	// significant bit: flag 1 enables location reporting and flag 2 the
	// recording of backlog location reports. The other flags are reserved,
	// 0.
	Flags uint8
	// Elements is the bit offset of the first type 5 element.
	// DecodeEnableDisable sets it; Encode ignores it.
	Elements int
}

// DecodeEnableDisable decodes the LOCATION REPORTING ENABLE/DISABLE
// REQUEST or RESPONSE PDU pdu. It returns ErrPDUType for another PDU and
// ErrShort for a PDU that ends early. (ETSI TS 100 392-18-1 clauses 6.2.14
// and 6.2.15)
func DecodeEnableDisable(pdu []byte) (EnableDisable, error) {
	r := reader{b: pdu}
	r.start(ExtensionEnableDisable, -1)
	e := EnableDisable{Response: r.flag()}
	if !e.Response {
		e.AckRequested = r.flag()
	}
	e.Flags = r.u8(8)
	if r.err != nil {
		return EnableDisable{}, r.err
	}
	e.Elements = r.at
	return e, nil
}

// Encode appends the PDU of e to dst, with 0 bits after its last bit, and
// returns dst and the number of bits of the PDU. A response does not code
// AckRequested. It returns dmrgo.ErrNilReceiver for a nil e, and allocates
// only to grow dst. (ETSI TS 100 392-18-1 clauses 6.2.14 and 6.2.15)
func (e *EnableDisable) Encode(dst []byte) ([]byte, int, error) {
	if e == nil {
		return dst, 0, dmrgo.ErrNilReceiver
	}
	var w writer
	w.start(ExtensionEnableDisable, -1)
	w.flag(e.Response)
	if !e.Response {
		w.flag(e.AckRequested)
	}
	w.put(8, uint64(e.Flags), nil)
	return w.finish(dst)
}
