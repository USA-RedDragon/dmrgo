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
// caller reuses that packet. The exceptions are the OutputOptions of
// Master.Accept and the OutputConfig that Master.Packet reports for an
// RPTK, which point into the Master as an OutputSend does.
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
// ends the session when the login does not finish within LoginTimeout of
// the first RPTL, and after Timeout without a packet once it is logged in.
//
// An acknowledgement does not name the step that it acknowledges. A
// repeater that sends its RPTK again because the acknowledgement is late
// gets two, takes the second as the acknowledgement of its RPTC, and
// sends its RPTO, pings and traffic before the consumer calls Accept.
// The Peer, MMDVMHost and DMRGateway all do this. While it waits for
// Accept, the Master drops those packets of the repeater without MSTNAK,
// keeps the RPTO text, and reports it after OutputUp, so the login of the
// repeater does not fail. In the same way, MMDVMHost and DMRGateway take
// a second salt, sent for a repeated RPTL, as the acknowledgement of their
// RPTK and send the RPTC at once; when that RPTC arrives before the RPTK,
// the Master keeps it and reports it after the good RPTK.
//
// (DMRGateway ee79d38 DMRNetwork.cpp clock; MMDVMHost 7008c62
// DMRDirectNetwork.cpp clock; DL5DI, G4KLX and DG1HT, IPSC Protocol Specs
// for homebrew DMR repeater, 2015-07-26; HBlink3 1db2730 hblink.py
// master_maintenance_loop; FreeDMR 546165eb hblink.py and
// bridge_master.py)
package dmrdsession
