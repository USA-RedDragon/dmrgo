// Package dmrdsession runs the sessions of the protocols of package dmrd: the
// login, keepalive, retry and close of the MMDVM network protocol
// (dmrd.DialectMMDVM) and of the HomeBrew Repeater Protocol
// (dmrd.DialectHBRP), on the repeater side (Peer) and on the master side
// (Master, one per repeater), and the signatures and keepalive of
// OpenBridge (Bridge).
//
// The machines do no IO. Each method takes the time now, on a monotonic
// clock that the caller picks, and the packets that the caller received,
// and appends outputs to a slice of the caller: packets to send, and
// events. A slice with capacity MaxOutputs never grows, and after the
// constructor no method allocates or panics. Deadline reports when a
// machine needs a Tick. An OutputSend packet points into the machine and
// is valid until the next call of the machine. The packet of OutputConfig,
// OutputOptions, OutputData and OutputMessage is not a copy: it points
// into the packet that the caller passed in, and is valid only until the
// caller reuses that packet.
//
// The Peer follows DMRGateway: it sends RPTL, RPTK with the SHA-256 token
// of the salt and the password, RPTC and, when it has options, RPTO, each
// after the acknowledgement of the one before, sends the step again every
// Retry, pings every Retry when logged in, and logs in again after Timeout
// without a reply, after MSTNAK and after MSTCL. The Master answers each
// step, sends OutputLogin and OutputConfig to the consumer before it
// answers the RPTL and the RPTC, takes the RPTO of a logged-in repeater
// with an RPTACK, as DMRGateway waits for it before it sends traffic,
// answers a packet from a repeater that is not logged in with MSTNAK, and
// ends the session after LoginTimeout without a packet during the login and
// Timeout after it.
//
// (DMRGateway ee79d38 DMRNetwork.cpp clock; MMDVMHost 7008c62
// DMRDirectNetwork.cpp clock; DL5DI, G4KLX and DG1HT, IPSC Protocol Specs
// for homebrew DMR repeater, 2015-07-26; HBlink3 1db2730 hblink.py
// master_maintenance_loop; FreeDMR 546165eb hblink.py and
// bridge_master.py)
package dmrdsession
