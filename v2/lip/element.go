package lip

import (
	ibits "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// ElementID is the type 5 element identifier of an optional information
// element. Values that have no constant are reserved, and 31 is the
// extended identifier, whose coding the spec does not define (ETSI TS 100
// 392-18-1 clause 6.3.86).
type ElementID uint8

// The ElementID values.
const (
	ElementDirectionAccuracy   ElementID = 0  // direction of travel and direction of travel accuracy
	ElementExtendedUserData    ElementID = 1  // extended user defined data
	ElementPositionAccuracy    ElementID = 2  // horizontal position and horizontal position accuracy
	ElementVelocityAccuracy    ElementID = 3  // horizontal velocity and horizontal velocity accuracy
	ElementDestination         ElementID = 4  // location information destination
	ElementAltitudeAccuracy    ElementID = 5  // location altitude and location altitude accuracy
	ElementMessageReference    ElementID = 6  // location message reference, 8 bits
	ElementMaxInformationAge   ElementID = 7  // maximum information age, 7 bits
	ElementMaxResponseTime     ElementID = 8  // maximum response time, 7 bits
	ElementDefaultLifetime     ElementID = 9  // default enable/disable lifetime, 2 bits
	ElementRequestPriority     ElementID = 11 // request priority, 2 bits
	ElementResultCode          ElementID = 12 // result code, 8 bits
	ElementSDSType1Value       ElementID = 13 // SDS type-1 value, 16 bits
	ElementStartTime           ElementID = 14 // start time, 22 bits
	ElementStatusValue         ElementID = 15 // status value, 16 bits
	ElementStopTime            ElementID = 16 // stop time, 22 bits
	ElementIdentification      ElementID = 17 // terminal or location identification
	ElementTriggerDefinition   ElementID = 19 // trigger definition
	ElementTriggerRemoval      ElementID = 20 // trigger removal
	ElementVerticalAccuracy    ElementID = 21 // vertical velocity and vertical velocity accuracy
	ElementTemporaryControl    ElementID = 22 // temporary control parameter definition
	ElementBacklogAvailable    ElementID = 23 // backlog information available, 10 bits
	ElementBacklogContinuation ElementID = 24 // backlog continuation state
)

const (
	elementExtended             = 31
	minElementHeader            = 11
	maxElementBits              = 134 * 8
	minExtendedElementBits      = 64
	extendedElementOctetsAdjust = 7
)

// String returns the name of the ElementID. It allocates only for a
// reserved value.
func (id ElementID) String() string {
	switch id {
	case ElementDirectionAccuracy:
		return "Direction of travel and direction of travel accuracy"
	case ElementExtendedUserData:
		return "Extended user defined data"
	case ElementPositionAccuracy:
		return "Horizontal position and horizontal position accuracy"
	case ElementVelocityAccuracy:
		return "Horizontal velocity and horizontal velocity accuracy"
	case ElementDestination:
		return "Location information destination"
	case ElementAltitudeAccuracy:
		return "Location altitude and location altitude accuracy"
	case ElementMessageReference:
		return "Location message reference"
	case ElementMaxInformationAge:
		return "Maximum information age"
	case ElementMaxResponseTime:
		return "Maximum response time"
	case ElementDefaultLifetime:
		return "Default enable/disable lifetime"
	case ElementRequestPriority:
		return "Request priority"
	case ElementResultCode:
		return "Result code"
	case ElementSDSType1Value:
		return "SDS type-1 value"
	case ElementStartTime:
		return "Start time"
	case ElementStatusValue:
		return "Status value"
	case ElementStopTime:
		return "Stop time"
	case ElementIdentification:
		return "Terminal or location identification"
	case ElementTriggerDefinition:
		return "Trigger definition"
	case ElementTriggerRemoval:
		return "Trigger removal"
	case ElementVerticalAccuracy:
		return "Vertical velocity and vertical velocity accuracy"
	case ElementTemporaryControl:
		return "Temporary control parameter definition"
	case ElementBacklogAvailable:
		return "Backlog information available"
	case ElementBacklogContinuation:
		return "Backlog continuation state"
	}
	return "ElementID(" + text.Int(int64(id)) + ")"
}

// Element is a type 5 information element of a LIP PDU, as NextElement
// finds it: its identifier and where its data lies in the PDU. (ETSI TS
// 100 392-18-1 clause 6.4)
type Element struct {
	// ID is the type 5 element identifier.
	ID ElementID
	// Offset is the bit offset of the element data in the PDU.
	Offset int
	// Bits is the length of the element data in bits. For a length of 8
	// octets or more it holds the fill bits too.
	Bits int
	// Next is the bit offset of the element after it.
	Next int
}

// NextElement reads the type 5 element that starts at bit at of the PDU
// pdu, such as the Elements offset of a decoded PDU or the Next of the
// element before. ok is false at the end of the PDU: fewer bits than an
// element header, or only 0 bits, follow at. It returns ErrShort for an
// element that ends after the PDU or an at out of range, and ErrElement
// for the extended identifier or the reserved length extension. (ETSI TS
// 100 392-18-1 clause 6.4)
func NextElement(pdu []byte, at int) (e Element, ok bool, err error) {
	total := len(pdu) * 8
	if at < 0 || at > total {
		return Element{}, false, ErrShort
	}
	if total-at < minElementHeader || zeros(pdu, at) {
		return Element{}, false, nil
	}
	r := reader{b: pdu, at: at}
	e.ID = ElementID(r.u8(5))
	e.Bits = int(r.u8(6))
	if e.Bits == 0 {
		ext := int(r.u8(7))
		if r.err == nil && ext == 0 {
			return Element{}, false, ErrElement
		}
		e.Bits = (ext + extendedElementOctetsAdjust) * 8
	}
	if r.err != nil {
		return Element{}, false, r.err
	}
	if e.ID == elementExtended {
		return Element{}, false, ErrElement
	}
	if r.at+e.Bits > total {
		return Element{}, false, ErrShort
	}
	e.Offset, e.Next = r.at, r.at+e.Bits
	return e, true, nil
}

func zeros(pdu []byte, at int) bool {
	total := len(pdu) * 8
	for at < total {
		n := min(total-at, 32)
		if ibits.Field(pdu, at, n) != 0 {
			return false
		}
		at += n
	}
	return true
}

// Uint returns the data of e in the PDU pdu as an unsigned integer, the
// first bit most significant, for an element of up to 64 bits, such as the
// location message reference or the result code. For a longer element it
// returns the first 64 bits, and 0 when pdu does not hold the data.
func (e Element) Uint(pdu []byte) uint64 {
	n := min(e.Bits, 64)
	if e.Offset < 0 || n <= 0 || e.Offset > len(pdu)*8 || n > len(pdu)*8-e.Offset {
		return 0
	}
	return ibits.Field64(pdu, e.Offset, n)
}

// AppendData appends the data of e in the PDU pdu to dst, packed the first
// bit in the most significant bit of the first octet and 0 bits after the
// last, and returns dst. It appends nothing when pdu does not hold the
// data. It allocates only to grow dst.
func (e Element) AppendData(dst, pdu []byte) []byte {
	if e.Offset < 0 || e.Bits <= 0 || e.Offset > len(pdu)*8 || e.Bits > len(pdu)*8-e.Offset {
		return dst
	}
	start := len(dst)
	for range (e.Bits + 7) / 8 {
		dst = append(dst, 0)
	}
	ibits.Copy(dst[start:], 0, pdu, e.Offset, e.Bits)
	return dst
}

// AppendElement adds a type 5 element with the identifier id and the
// first n bits of data, packed the first bit most significant, at bit
// offset bits of the PDU dst, such as the output of an Encode. It
// returns dst, with 0 bits after its new last bit, and the number of bits
// of the PDU. Data of 64 bits or more is coded in octets with 1 fill bits
// at the end. It returns ErrElement for an n out of 1 to 1072 or longer
// than data, or an id over 30, and ErrShort for a dst shorter than bits.
// It allocates only to grow dst. (ETSI TS 100 392-18-1 clause 6.4)
func AppendElement(dst []byte, bits int, id ElementID, data []byte, n int) ([]byte, int, error) {
	if id >= elementExtended || n < 1 || n > maxElementBits || n > len(data)*8 {
		return dst, bits, ErrElement
	}
	if bits < 0 || bits > len(dst)*8 {
		return dst, bits, ErrShort
	}
	size, header := n, minElementHeader
	if n >= minExtendedElementBits {
		size = (n + 7) / 8 * 8
		header += 7
	}
	dst = dst[:(bits+7)/8]
	if bits%8 != 0 {
		dst[len(dst)-1] &^= 0xFF >> (bits % 8)
	}
	end := bits + header + size
	for len(dst) < (end+7)/8 {
		dst = append(dst, 0)
	}
	ibits.SetField(dst, bits, 5, uint64(id))
	if n < minExtendedElementBits {
		ibits.SetField(dst, bits+5, 6, uint64(n))
	} else {
		ibits.SetField(dst, bits+5, 6, 0)
		ibits.SetField(dst, bits+11, 7, uint64(size/8-extendedElementOctetsAdjust))
	}
	ibits.Copy(dst, bits+header, data, 0, n)
	for i := bits + header + n; i < end; i++ {
		ibits.SetField(dst, i, 1, 1)
	}
	return dst, end, nil
}
