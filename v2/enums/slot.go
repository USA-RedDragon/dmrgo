package enums

// Slot is a TDMA channel of a carrier, also called a time slot. The values
// match the Channel field of a framer burst. The zero value is SlotUnknown.
// (ETSI TS 102 361-1 clause 5.1.1)
type Slot uint8

// The Slot values.
const (
	// SlotUnknown is a burst whose TDMA channel is not known.
	SlotUnknown Slot = iota

	Slot1 // TDMA channel 1
	Slot2 // TDMA channel 2
)

// Other returns the other TDMA channel of the carrier: Slot2 for Slot1 and
// Slot1 for Slot2. It returns SlotUnknown for any other value.
func (s Slot) Other() Slot {
	switch s {
	case Slot1:
		return Slot2
	case Slot2:
		return Slot1
	case SlotUnknown:
	}
	return SlotUnknown
}

// String returns "TS1", "TS2" or "Unknown".
func (s Slot) String() string {
	switch s {
	case Slot1:
		return "TS1"
	case Slot2:
		return "TS2"
	case SlotUnknown:
	}
	return unknownName
}
