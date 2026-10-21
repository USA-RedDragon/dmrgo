package trunk

// Status is the 7-bit value of the status call service. Values 0 to 99
// have a meaning that the users agree, 100 to 122 are reserved, and the
// constants have the meaning that the standard gives them (ETSI TS 102
// 361-4 clause 6.6.6.0).
type Status uint8

// The Status values with a meaning in the standard.
const (
	// StatusTerminate asks the TSCC to stop the transmission of an MS and
	// to end its call, such as an ALS call (ETSI TS 102 361-4 clause
	// 6.4.14.3.4).
	StatusTerminate Status = 123
	// StatusCancelEmergency cancels an emergency alarm (ETSI TS 102 361-4
	// clause 6.6.6.3.2).
	StatusCancelEmergency Status = 124
	// StatusInterrupt asks the TSCC to stop the transmission of an MS so
	// that the caller can talk (ETSI TS 102 361-4 clause 6.4.14.3.3).
	StatusInterrupt Status = 125
	// StatusEmergency is an emergency alarm (ETSI TS 102 361-4 clause
	// 6.6.6.3.1).
	StatusEmergency Status = 126
	// StatusPoll asks the target for its status (ETSI TS 102 361-4 clause
	// 6.6.6.2).
	StatusPoll Status = 127
)

// StunAction is a supplementary service of a TSCC that disables or enables
// an MS (ETSI TS 102 361-4 clauses 6.4.9 and 6.4.10).
type StunAction uint8

// The StunAction values.
const (
	// StunNone is the zero StunAction.
	StunNone StunAction = iota
	// StunStun denies the MS the user services.
	StunStun
	// StunRevive gives a stunned MS its user services again.
	StunRevive
	// StunKill disables the MS for good.
	StunKill
)

// String returns the name of the StunAction.
func (a StunAction) String() string {
	switch a {
	case StunNone:
		return "None"
	case StunStun:
		return stunName
	case StunRevive:
		return "Revive"
	case StunKill:
		return "Kill"
	}
	return unknownName
}

// Authenticator gives the 24-bit response of the challenge and response
// authentication, as the RC4 algorithm with the key K (and the PSN, when
// it is used) of an MS computes it. An MS uses it to authenticate the
// TSCC before a stun, revive or kill, and a TSCC to answer the challenge
// of the MS (ETSI TS 102 361-4 clauses 6.4.8, 6.4.9.2 and 6.4.10).
type Authenticator interface {
	// Response returns the response of the MS addr to challenge. ok is
	// false when there is no key for addr.
	Response(addr, challenge uint32) (response uint32, ok bool)
}
