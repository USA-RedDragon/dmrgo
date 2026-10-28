// Package rf recovers DMR bursts from the I/Q samples of a software radio
// and from the output of an FM discriminator: 4FSK detection, SYNC search
// and slot timing, soft-decision decoding of each FEC field, and the
// superframe position of voice bursts.
//
// NewReceiver makes a Receiver from an RxConfig, for I/Q at RxConfig.Rate
// in RxConfig.Format: CU8 as rtl_sdr writes it, CS8 as hackrf_transfer
// writes it, CS16 or CF32. NewAudioReceiver makes an AudioReceiver from an
// AudioRxConfig, for the samples of a discriminator. Init sets up either
// one in Tier0 in memory of the caller, for a microcontroller.
//
// The callback of the config picks the mode. With OnBursts set, Write
// takes samples and hands the bursts that they complete to OnBursts, and
// Flush ends the stream. With OnBursts nil, AppendBursts appends the
// bursts to a slice, and AppendFlush ends the stream.
//
//	rx, err := rf.NewReceiver(&rf.RxConfig{Tier: rf.Tier2, Rate: 2048000, Format: rf.IQFormatCU8})
//	if err != nil {
//		return err
//	}
//	var burst layer2.Burst
//	bursts := rx.AppendBursts(nil, iq)
//	bursts = rx.AppendFlush(bursts)
//	for i := range bursts {
//		data, _ := bursts[i].Decoded()
//		err := burst.DecodeFromBytes(data)
//		...
//	}
//
// A Monitor follows the TDMA channels of the bursts, as a scanner does: it
// finds the TDMA channel of each burst from the CACHs of its stream,
// decodes it, and keeps the LC of the call on each channel. Burst.Hint
// gives the layer2.Hint of a burst for the voice and data machines.
//
// The Tier of the config sets the detection and its cost, from Tier0, which
// fits a microcontroller, to Tier3, for offline decoding. The constructors
// allocate every buffer of the receiver, and Write and Flush allocate
// nothing. The README gives the sensitivity, compute and RAM of each tier.
// (ETSI TS 102 361-1 clauses 6 and 10)
package rf
