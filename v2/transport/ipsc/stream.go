package ipsc

// Stream fills the fields that the user packets of one call share or count,
// as a MOTOTRBO repeater sends them. Set CallSeq, CallControl and the first
// Seq and Timestamp, then call Fill for each packet of the call in order.
// The zero Stream is ready to use. It is not safe for concurrent use.
type Stream struct {
	// CallSeq is octet 5 of every packet of the call. An XPR 8400 counts it
	// up by one for each call.
	CallSeq uint8
	// CallControl identifies the call in every packet.
	CallControl uint32
	// Seq is the RTP sequence number of the next packet.
	Seq uint16
	// Timestamp is the RTP timestamp of the next packet.
	Timestamp uint32
	// Sent is the number of packets that Fill has filled.
	Sent uint32
}

// Fill sets CallSeq, CallControl and the RTP header of u: Flags 0x80, the
// marker bit on the first packet of the call, PayloadType 0x5D, or 0x5E
// with the end flag of Info on the last packet, and Seq and Timestamp. It
// then counts Seq up by 1 and Timestamp by 480, one burst. Skip counts up
// for a lost burst. A nil s or u is a no-op.
func (s *Stream) Fill(u *User, last bool) {
	if s == nil {
		return
	}

	if s == nil || u == nil {
		return
	}
	u.CallSeq, u.CallControl = s.CallSeq, s.CallControl
	u.RTP = RTP{Flags: 0x80, Marker: s.Sent == 0, PayloadType: 0x5D, Seq: s.Seq, Timestamp: s.Timestamp, SSRC: u.RTP.SSRC}
	if last {
		u.RTP.PayloadType = 0x5E
		u.Info.SetEnd(true)
	}
	s.Sent++
	s.Skip(1)
}

// Skip counts Seq up by n and Timestamp by 480 for each of n bursts that
// were lost, so that the next packet shows the gap. A nil s is a no-op.
func (s *Stream) Skip(n uint16) {
	if s == nil {
		return
	}
	s.Seq += n
	s.Timestamp += 480 * uint32(n)
}
