package enums

// Access is the channel access policy of a transmission (ETSI TS 102 361-1
// clause 5.2.1.6). The zero value is AccessPoliteOwnCC.
type Access uint8

// The Access values.
const (
	// AccessPoliteOwnCC waits while the logical channel carries a
	// transmission with the own color code.
	AccessPoliteOwnCC Access = iota
	// AccessPoliteAll waits while the channel has any activity, DMR or
	// other.
	AccessPoliteAll
	// AccessImpolite transmits whatever the activity on the channel.
	AccessImpolite
)

// String returns the clause 5.2.1.6 name of the policy.
func (a Access) String() string {
	switch a {
	case AccessPoliteOwnCC:
		return "Polite to own Colour Code"
	case AccessPoliteAll:
		return "Polite to all"
	case AccessImpolite:
		return "Impolite"
	}
	return unknownName
}
