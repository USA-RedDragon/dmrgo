package dmrd

import "github.com/USA-RedDragon/dmrgo/v2"

// Bridge is an OpenBridge data packet between two masters. OpenBridge has
// no login: each packet carries its own signature, which package auth
// computes and checks.
//
// Version 1 is the form of BrandMeister, HBlink3, DMRHub and FreeDMR without
// enhanced OpenBridge: the 53-octet DMRD with the network ID in the
// repeater field and the slot bit clear, then a 20-octet HMAC-SHA1
// signature (73 octets). Versions 4 and 5 are the DMRE packets of FreeDMR's
// enhanced OpenBridge (85 and 89 octets): DMRD with BER and RSSI, the
// version, the send time, the source server, in version 5 the source
// repeater, the hop count and a 16-octet keyed BLAKE2b hash.
type Bridge struct {
	// Version is 1, 4 or 5: the 73, 85 or 89-octet form.
	Version uint8
	// Data is the DMRD packet. Its Size is 53 for version 1 and 55, with
	// BER and RSSI, for versions 4 and 5.
	Data Data
	// Time is the send time in nanoseconds since 1970 (versions 4 and 5).
	Time uint64
	// SourceServer is the ID of the server that sent the call into
	// OpenBridge (versions 4 and 5).
	SourceServer uint32
	// SourceRepeater is the ID of the repeater that the call came from
	// (version 5).
	SourceRepeater uint32
	// Hops is the number of OpenBridge links the call has crossed
	// (versions 4 and 5).
	Hops uint8
	// Signature is the HMAC-SHA1 signature of version 1, or in its first 16
	// octets the BLAKE2b hash of versions 4 and 5.
	Signature [20]byte
}

// DecodeBridge decodes the OpenBridge data packet p: a 73-octet DMRD or an
// 85 or 89-octet DMRE packet. It returns ErrCommand when p is neither,
// ErrShort or ErrLength for another length, ErrSignature when the version
// octet of a DMRE packet is not 4 for 85 octets or 5 for 89, and the errors
// of DecodeData.
// It does not check the signature (see package auth) and does not
// allocate.
func DecodeBridge(p []byte) (Bridge, error) {
	switch {
	case hasTag(p, "DMRD"):
		if len(p) < 73 {
			return Bridge{}, ErrShort
		}
		if len(p) != 73 {
			return Bridge{}, ErrLength
		}
		d, err := DecodeData(p[:53])
		if err != nil {
			return Bridge{}, err
		}
		b := Bridge{Version: 1, Data: d}
		copy(b.Signature[:], p[53:])
		return b, nil
	case hasTag(p, "DMRE"):
		switch len(p) {
		case 85:
			m, err := decodeDmreV4(p)
			if err != nil {
				return Bridge{}, err
			}
			if m.Version != 4 {
				return Bridge{}, ErrSignature
			}
			b := Bridge{Version: m.Version, Time: m.Time, SourceServer: m.SourceServer, Hops: m.Hops}
			b.Data = Data{Seq: m.Seq, Source: m.Source, Destination: m.Destination, Repeater: m.Repeater, Slot: m.Slot, Private: m.Private,
				Frame: m.Frame, Value: m.Value, Stream: m.Stream, Burst: m.Burst, BER: m.BER, RSSI: m.RSSI, Size: 55}
			copy(b.Signature[:], m.Hash[:])
			return b, nil
		case 89:
			m, err := decodeDmreV5(p)
			if err != nil {
				return Bridge{}, err
			}
			if m.Version != 5 {
				return Bridge{}, ErrSignature
			}
			b := Bridge{Version: m.Version, Time: m.Time, SourceServer: m.SourceServer, SourceRepeater: m.SourceRepeater, Hops: m.Hops}
			b.Data = Data{Seq: m.Seq, Source: m.Source, Destination: m.Destination, Repeater: m.Repeater, Slot: m.Slot, Private: m.Private,
				Frame: m.Frame, Value: m.Value, Stream: m.Stream, Burst: m.Burst, BER: m.BER, RSSI: m.RSSI, Size: 55}
			copy(b.Signature[:], m.Hash[:])
			return b, nil
		}
		if len(p) < 85 {
			return Bridge{}, ErrShort
		}
		return Bridge{}, ErrLength
	}
	return Bridge{}, ErrCommand
}

// AppendBinary appends the packet b to dst in the form of b.Version, with
// b.Signature as it is: sign the packet afterward with package auth. It
// returns dst unchanged and an error for another version, when Data.Size
// does not fit the version (0 or 53 for version 1, 0 or 55 for 4 and 5), or
// with the errors of Data.AppendBinary. A nil b gives ErrCommand. It
// allocates only to grow dst.
func (b *Bridge) AppendBinary(dst []byte) ([]byte, error) {
	if b == nil {
		return dst, ErrCommand
	}
	d := b.Data
	switch b.Version {
	case 1:
		if d.Size != 0 && d.Size != 53 {
			return dst, errBridgeData
		}
		n := len(dst)
		var err error
		dst, err = d.AppendBinary(dst)
		if err != nil {
			return dst[:n], err
		}
		return append(dst, b.Signature[:]...), nil
	case 4:
		if d.Size != 0 && d.Size != 55 {
			return dst, errBridgeData
		}
		m := dmreV4{Seq: d.Seq, Source: d.Source, Destination: d.Destination, Repeater: d.Repeater, Slot: d.Slot, Private: d.Private,
			Frame: d.Frame, Value: d.Value, Stream: d.Stream, Burst: d.Burst, BER: d.BER, RSSI: d.RSSI,
			Version: b.Version, Time: b.Time, SourceServer: b.SourceServer, Hops: b.Hops}
		copy(m.Hash[:], b.Signature[:])
		return m.AppendBinary(dst)
	case 5:
		if d.Size != 0 && d.Size != 55 {
			return dst, errBridgeData
		}
		m := dmreV5{Seq: d.Seq, Source: d.Source, Destination: d.Destination, Repeater: d.Repeater, Slot: d.Slot, Private: d.Private,
			Frame: d.Frame, Value: d.Value, Stream: d.Stream, Burst: d.Burst, BER: d.BER, RSSI: d.RSSI,
			Version: b.Version, Time: b.Time, SourceServer: b.SourceServer, SourceRepeater: b.SourceRepeater, Hops: b.Hops}
		copy(m.Hash[:], b.Signature[:])
		return m.AppendBinary(dst)
	}
	return dst, errBridgeVersion
}

// BridgeControl is a control packet of FreeDMR's enhanced OpenBridge, each
// with a 20-octet HMAC-SHA1 signature: BCKA (keepalive, every 10 s), BCSQ
// (the receiver asks the sender to stop a stream to a talkgroup), BCST (stun:
// stop all traffic) and BCVE (the protocol version, every 60 s).
type BridgeControl struct {
	// Command is CommandBCKA, CommandBCSQ, CommandBCST or CommandBCVE.
	Command Command
	// TalkGroup is the talkgroup of a BCSQ (24 bits).
	TalkGroup uint32
	// Stream is the stream ID of a BCSQ.
	Stream uint32
	// Version is the version of a BCVE.
	Version uint8
	// Signature is the HMAC-SHA1 signature.
	Signature [20]byte
}

// DecodeBridgeControl decodes the FreeDMR OpenBridge control packet p. It
// returns ErrCommand when p is not one, and ErrShort and ErrLength for a
// wrong length. It does not check the signature and does not allocate.
func DecodeBridgeControl(p []byte) (BridgeControl, error) {
	switch {
	case hasTag(p, "BCKA"), hasTag(p, "BCST"):
		m, err := decodeBcTag(p)
		if err != nil {
			return BridgeControl{}, err
		}
		c := BridgeControl{Command: CommandBCKA, Signature: m.Signature}
		if hasTag(p, "BCST") {
			c.Command = CommandBCST
		}
		return c, nil
	case hasTag(p, "BCSQ"):
		m, err := decodeBcsq(p)
		if err != nil {
			return BridgeControl{}, err
		}
		return BridgeControl{Command: CommandBCSQ, TalkGroup: m.TalkGroup, Stream: m.Stream, Signature: m.Signature}, nil
	case hasTag(p, "BCVE"):
		m, err := decodeBcve(p)
		if err != nil {
			return BridgeControl{}, err
		}
		return BridgeControl{Command: CommandBCVE, Version: m.Version, Signature: m.Signature}, nil
	}
	return BridgeControl{}, ErrCommand
}

// AppendBinary appends the packet c to dst, with c.Signature as it is. It
// returns dst unchanged and an error for another Command, and a
// *dmrgo.FieldRangeError when TalkGroup does not fit in 24 bits, and
// dmrgo.ErrNilReceiver for a nil c. It allocates only to grow dst.
func (c *BridgeControl) AppendBinary(dst []byte) ([]byte, error) {
	if c == nil {
		return dst, dmrgo.ErrNilReceiver
	}
	return c.appendBinary(dst)
}

func (c BridgeControl) appendBinary(dst []byte) ([]byte, error) {
	switch c.Command {
	case CommandBCKA, CommandBCST:
		m := bcTag{Signature: c.Signature}
		copy(m.Tag[:], c.Command.tag())
		return m.AppendBinary(dst)
	case CommandBCSQ:
		m := bcsq{TalkGroup: c.TalkGroup, Stream: c.Stream, Signature: c.Signature}
		return m.AppendBinary(dst)
	case CommandBCVE:
		m := bcve{Version: c.Version, Signature: c.Signature}
		return m.AppendBinary(dst)
	default:
	}
	return dst, errDialectCommand
}
