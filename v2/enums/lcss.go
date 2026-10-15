package enums

// ETSI TS 102 361-1 - 9.3.3 LC Start/Stop (LCSS)
type LCSS uint8

const (
	LCSSSingleFragment LCSS = iota
	LCSSFirstFragment
	LCSSLastFragment
	LCSSContinuationFragment
)

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
	return unknownName
}
