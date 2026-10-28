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
// slice of the caller. A slice with capacity MaxOutputs for a Follower, MS
// or CapacityPlusFollower, or MaxTSCCOutputs for a TSCC, never grows, and
// after the constructor no method allocates. Deadline reports when a machine
// needs a Tick. now is the start of the received burst; Due is the start
// of the burst to send. On the slot grid of ETSI TS 102 361-1 clauses 4.2
// and 5.1.1 and Figure 5.23, inbound time slot 1 or 2 starts 30 ms after
// outbound time slot 1 or 2. The inbound slot of a logical channel starts
// with its outbound slot on an aligned timing channel, and 30 ms after it
// on an offset timing channel, as the C_ALOHA of the TSCC and the grant of
// a payload channel give (Table 5.1). An MS sends an answer in the next
// TDMA frame after the end of the PDU that requests it, and the AT bit of
// the CACH 30 ms before an inbound slot shows if it is withdrawn from
// random access (ETSI TS 102 361-1 clause 4.5, ETSI TS 102 361-4 clause
// 6.2.1.1.3).
//
// The consumer owns the radio. An OutputTune requests that it tune to a channel
// number, with the frequencies from the ChannelPlan when they are known.
// The machines get the plan from the CG_AP blocks of grants and from
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
