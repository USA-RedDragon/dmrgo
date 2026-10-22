package ipscsession

import "github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth"

// MaxOutputOctets returns the most octets that the packets of the outputs
// of one call of the Master hold, where outputs that share a packet count
// it once: the packet that the call builds, the body of an OutputData and
// the de-registration of each peer.
func (m *Master) MaxOutputOctets() int {
	if m == nil {
		return 0
	}

	return 2*MaxPacket + m.maxPeers*(5+ipscauth.TrailerLen)
}

// MaxOutputOctets returns the most octets that the packets of the outputs
// of one call of the Peer hold, where outputs that share a packet count it
// once: the body of an OutputData and the four control packets that the
// Peer builds.
func (p *Peer) MaxOutputOctets() int {
	if p == nil {
		return 0
	}

	return MaxPacket + 4*MaxControl
}
