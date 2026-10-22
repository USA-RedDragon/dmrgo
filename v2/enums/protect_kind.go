package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// ProtectKind is the 3-bit Protect_Kind of a P_PROTECT. Values without a
// constant are reserved. (ETSI TS 102 361-4 V1.12.1 clause 7.2.21,
// Table 7.82)
type ProtectKind uint8

// The ProtectKind values.
const (
	ProtectDisablePTT      ProtectKind = 0b000 // DIS_PTT: the addressed MSs must not transmit
	ProtectEnablePTT       ProtectKind = 0b001 // EN_PTT: the addressed MSs may transmit again
	ProtectIllegallyParked ProtectKind = 0b010 // ILLEGALLY_PARKED: MSs other than the source and target must leave the payload channel
	ProtectEnablePTTOneMS  ProtectKind = 0b011 // EN_PTT_ONE_MS: only the addressed MS may transmit
)

// String returns the alias of the ProtectKind. It allocates only for a
// value without a constant.
func (k ProtectKind) String() string {
	switch k {
	case ProtectDisablePTT:
		return "DIS_PTT"
	case ProtectEnablePTT:
		return "EN_PTT"
	case ProtectIllegallyParked:
		return "ILLEGALLY_PARKED"
	case ProtectEnablePTTOneMS:
		return "EN_PTT_ONE_MS"
	}
	return "ProtectKind(" + text.Int(int64(k)) + ")"
}
