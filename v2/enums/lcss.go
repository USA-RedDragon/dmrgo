package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// LCSS is the 2-bit LC Start/Stop of an EMB or CACH. It shows whether a
// fragment starts, continues or ends LC or CSBK signalling (ETSI TS 102
// 361-1 clause 9.3.3, Table 9.20).
type LCSS uint8

// The LCSS values.
const (
	LCSSSingleFragment       LCSS = iota // single fragment LC or first fragment CSBK
	LCSSFirstFragment                    // first fragment LC
	LCSSLastFragment                     // last fragment LC or CSBK
	LCSSContinuationFragment             // continuation fragment LC or CSBK
)

// String returns the name of the LCSS. It allocates only for a value over
// 3.
func (lcss LCSS) String() string {
	switch lcss {
	case LCSSSingleFragment:
		return "Single fragment LC or first fragment CSBK"
	case LCSSFirstFragment:
		return "First fragment LC"
	case LCSSLastFragment:
		return "Last fragment LC or CSBK"
	case LCSSContinuationFragment:
		return "Continuation fragment LC or CSBK"
	}
	return "LCSS(" + text.Int(int64(lcss)) + ")"
}
