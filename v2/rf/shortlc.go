package rf

import "github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"

// ShortLC is a Short LC that a receiver decoded from the CACH of four
// bursts (ETSI TS 102 361-1 clause 7.1.3, ETSI TS 102 361-2 clause 7.1.3).
type ShortLC struct {
	// LC is the Short LC. LC.FEC is the result of the decode that
	// recovered it, and is never uncorrectable.
	LC pdu.ShortLC
	// Stream, Slot and Seconds are those of the burst whose CACH carries
	// the last fragment.
	Stream  int
	Slot    int64
	Seconds float64
	// Soft is set when soft-decision decoding recovered the Short LC from
	// fragments that the hard decode of layer2.ShortLCAssembler rejects.
	Soft bool
}
