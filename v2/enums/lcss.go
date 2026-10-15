package enums

// ETSI TS 102 361-1 - 9.3.3 LC Start/Stop (LCSS)
type LCSS uint8

const (
	SingleFragmentLCorCSBK LCSS = iota
	FirstFragmentLC
	LastFragmentLCorCSBK
	ContinuationFragmentLCorCSBK
)

func (lcss LCSS) String() string {
	switch lcss {
	case SingleFragmentLCorCSBK:
		return "Single fragment LC or CSBK"
	case FirstFragmentLC:
		return "First fragment LC"
	case LastFragmentLCorCSBK:
		return "Last fragment LC or CSBK"
	case ContinuationFragmentLCorCSBK:
		return "Continuation fragment LC or CSBK"
	}
	return unknownName
}
