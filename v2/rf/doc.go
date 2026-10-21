// Package rf recovers DMR bursts from the I/Q samples of a software radio:
// 4FSK detection, SYNC search and slot timing, soft-decision decoding of each
// FEC field, and the superframe position of voice bursts.
//
// New makes a Receiver from a Config. Write takes interleaved 8-bit I/Q at
// Config.Rate, as rtl_sdr and hackrf_transfer write it, and WriteBaseband
// takes complex baseband at BasebandRate. Both return the bursts that are
// complete, or hand each one to Config.OnBurst. Flush ends the stream.
//
//	rx, err := rf.New(&rf.Config{Rate: 2.048e6, Tier: rf.Tier2})
//	if err != nil {
//		return err
//	}
//	bursts, err := rx.Write(iq)
//	if err != nil {
//		return err
//	}
//	for _, b := range bursts {
//		data, _ := b.Decoded()
//		burst, err := layer2.NewBurstFromBytes(data)
//		...
//	}
//
// A Monitor follows the TDMA channels of the bursts, as a scanner does: it
// finds the TDMA channel of each burst from the CACHs of its stream,
// decodes it, and keeps the LC of the call on each channel. Burst.Hint
// gives the layer2.Hint of a burst for the voice and data machines.
//
// The Tier of the Config sets the detection and its cost, from Tier0, which
// fits a microcontroller, to Tier3, for offline decoding. New allocates every
// buffer of the Receiver, and Write, WriteBaseband and Flush allocate
// nothing. The README gives the sensitivity, compute and RAM of each tier.
// (ETSI TS 102 361-1 clauses 6 and 10)
package rf
