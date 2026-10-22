// Package trunk follows, takes part in and runs DMR Tier III trunked
// sites: the channel tables of a site, a Follower that follows the calls
// of a site without transmitting, an MS that registers and makes calls by
// random access, and a TSCC that runs a single site with its payload
// channels (ETSI TS 102 361-4 clauses 6.2 to 6.6). A CapacityPlusFollower
// follows a MOTOTRBO Capacity Plus or Linked Capacity Plus site in the same
// way, from its rest channel.
//
// The machines are state machines without I/O. Each method takes the time
// now, on a monotonic clock that the caller picks, and appends outputs to a
// slice of the caller. A slice with capacity MaxOutputs never grows, and
// after the constructor no method allocates. Deadline tells when a machine
// needs a Tick.
//
// The consumer owns the radio. An OutputTune asks it to tune to a channel
// number, with the frequencies from the ChannelPlan when they are known.
// The machines learn the plan from the CG_AP blocks of grants and from
// C_BCAST Chan_Freq, and a consumer adds the plan of its site with
// ChannelPlan.Add and ChannelPlan.AddRule. Trunked voice can start without
// a Voice LC Header, so a consumer that decodes the voice of a grant passes
// Grant.Call to voice.Receiver.Expect and feeds the bursts of the granted
// slot to that Receiver. An MS sends the bursts of OutputBurst at their
// Due time, and the voice of its own calls with a voice.Transmitter. A
// TSCC takes the inbound bursts of its carriers and keeps a 30 ms slot
// clock from its first Tick: a Tick at or after Deadline sends one burst
// with its CACH per carrier for the slot that is due, with the start of
// the slot in TSCCOutput.Due.
//
// An MS also uses the UDT services: UDT short data, UDT and USBD polling,
// call diversion and DGNA, and makes full-duplex and Ambient Listening
// Service calls (ETSI TS 102 361-4 clauses 6.6.2.6 and 6.6.4 to 6.6.11).
// Appended reads and writes the appended data of a UDT in each format of
// ETSI TS 102 361-4 Annex B.3.
package trunk
