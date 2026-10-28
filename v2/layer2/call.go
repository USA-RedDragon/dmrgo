package layer2

// Call is the parties of the call that a burst belongs to, as a machine
// Output gives them to a transport: the source and destination of a DMRD,
// IPSC or Hytera packet and its group or unit to unit flag.
type Call struct {
	// Group reports whether Target is a talkgroup rather than a radio.
	Group bool
	// Target is the 24-bit talkgroup or radio address.
	Target uint32
	// Source is the 24-bit address of the radio that sends.
	Source uint32
}
