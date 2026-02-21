package enums

// BurstType is the type of a DMR burst.
// ETSI TS 102 361-1 V2.5.1 (2017-10) - 6. Layer 2 burst format (sections 6.1 and 6.2)
type BurstType int

const (
	BurstTypeUnknown BurstType = iota - 1
	BurstTypeVocoder
	BurstTypeDataAndControl
)
