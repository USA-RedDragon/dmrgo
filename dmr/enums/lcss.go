package enums

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.3 LC Start/Stop (LCSS)
type LCSS int

const (
	SingleFragmentLCorCSBK LCSS = iota
	FirstFragmentLC
	LastFragmentLCorCSBK
	ContinuationFragmentLCorCSBK
)

func LCSSToName(lcss LCSS) string {
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
	return "Unknown"
}

func LCSSFromInt(i int) LCSS {
	switch LCSS(i) {
	case SingleFragmentLCorCSBK:
		return SingleFragmentLCorCSBK
	case FirstFragmentLC:
		return FirstFragmentLC
	case LastFragmentLCorCSBK:
		return LastFragmentLCorCSBK
	case ContinuationFragmentLCorCSBK:
		return ContinuationFragmentLCorCSBK
	}
	return SingleFragmentLCorCSBK
}
