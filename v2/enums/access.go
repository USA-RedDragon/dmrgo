package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// Access is the channel access policy of a transmission. The zero value is
// AccessPoliteOwnCC. (ETSI TS 102 361-1 clause 5.2.1.6)
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

// String returns the clause 5.2.1.6 name of the policy. It allocates only
// for a value without a constant.
func (a Access) String() string {
	switch a {
	case AccessPoliteOwnCC:
		return "Polite to own Color Code"
	case AccessPoliteAll:
		return "Polite to all"
	case AccessImpolite:
		return "Impolite"
	}
	return "Access(" + text.Int(int64(a)) + ")"
}
