package data

// MaxOutputOctets returns the most octets that the Message.Payload of the
// events of one call of the Receiver hold: one packet completes in a call,
// with a payload of at most ReceiverConfig.MaxFragment octets, or 48 for a
// UDT, and the message that it completes holds at most
// ReceiverConfig.MaxMessage octets.
func (r *Receiver) MaxOutputOctets() int {
	if r == nil {
		return 0
	}
	return max(r.maxFrag, len(r.u.buf)) + len(r.chain.buf)
}
