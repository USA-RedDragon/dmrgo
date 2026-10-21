package mmdvm

// Version is the payload of a CommandGetVersion reply. Protocol 1 (MMDVM_HS)
// sends only the description; protocol 2 (MMDVM) adds the capabilities,
// the CPU and the 16-octet unique ID.
type Version struct {
	// Protocol is the protocol version, 1 or 2.
	Protocol uint8
	// Capabilities is the set of modes; CapabilitiesV1 for protocol 1.
	Capabilities Capabilities
	// CPU is the processor (protocol 2).
	CPU CPU
	// UDID is the unique ID of the processor (protocol 2). An ST-Micro CPU
	// fills only the first 12 octets.
	UDID [16]byte
	// Description is the firmware and hardware text, which MMDVM_HS may
	// follow with a 0 octet and its unique ID as text. A decode points it
	// into the payload.
	Description []byte
}

// DecodeVersion decodes the payload of a CommandGetVersion reply. A
// protocol other than 1 and 2 keeps everything after the protocol octet in
// Description. It returns ErrShort.
func DecodeVersion(p []byte) (Version, error) {
	if len(p) < 1 {
		return Version{}, ErrShort
	}
	v := Version{Protocol: p[0]}
	if v.Protocol != 2 {
		if v.Protocol == 1 {
			v.Capabilities = CapabilitiesV1
		}
		v.Description = p[1:]
		return v, nil
	}
	if len(p) < 20 {
		return Version{}, ErrShort
	}
	v.Capabilities = Capabilities(p[1]) | Capabilities(p[2])<<8
	v.CPU = CPU(p[3])
	copy(v.UDID[:], p[4:20])
	v.Description = p[20:]
	return v, nil
}

// AppendBinary appends the payload to dst: for protocol 2 the protocol,
// the capabilities, the CPU, the UDID and the description, else the
// protocol and the description. It allocates only to grow dst and never
// fails.
func (v Version) AppendBinary(dst []byte) ([]byte, error) {
	dst = append(dst, v.Protocol)
	if v.Protocol == 2 {
		dst = append(dst, byte(v.Capabilities&0xFF), byte(v.Capabilities>>8), byte(v.CPU))
		dst = append(dst, v.UDID[:]...)
	}
	return append(dst, v.Description...), nil
}

// Debug is a debug frame from the modem: CommandDebug1 to CommandDebug5,
// a text and 0 to 4 big-endian numbers, or CommandDebugDump, raw octets.
type Debug struct {
	// Command is CommandDebug1 to CommandDebug5 or CommandDebugDump.
	Command Command
	// Text is the text, or the octets of a dump. A decode points it into
	// the payload.
	Text []byte
	// N is the numbers; Count tells how many are used.
	N [4]int16
}

// Count returns the number of numbers: 0 for CommandDebug1 and
// CommandDebugDump, up to 4 for CommandDebug5.
func (d Debug) Count() int {
	if d.Command >= CommandDebug1 && d.Command <= CommandDebug5 {
		return int(d.Command - CommandDebug1)
	}
	return 0
}

// DecodeDebug decodes a debug frame. It returns ErrCommand for another
// command, and ErrShort for a payload shorter than its numbers.
func DecodeDebug(f Frame) (Debug, error) {
	if f.Command != CommandDebugDump && (f.Command < CommandDebug1 || f.Command > CommandDebug5) {
		return Debug{}, ErrCommand
	}
	d := Debug{Command: f.Command}
	n := d.Count()
	t := len(f.Payload) - 2*n
	if t < 0 {
		return Debug{}, ErrShort
	}
	d.Text = f.Payload[:t]
	for i := range n {
		d.N[i] = int16(f.Payload[t+2*i])<<8 | int16(f.Payload[t+2*i+1])
	}
	return d, nil
}

// AppendBinary appends the payload to dst: the text and the numbers. Wrap
// it with AppendFrame and Command. It returns dst unchanged and an error
// when Command is not a debug command. It allocates only to grow dst.
func (d Debug) AppendBinary(dst []byte) ([]byte, error) {
	if d.Command != CommandDebugDump && (d.Command < CommandDebug1 || d.Command > CommandDebug5) {
		return dst, errDebugCommand
	}
	dst = append(dst, d.Text...)
	for i := range d.Count() {
		dst = append(dst, byte(d.N[i]>>8&0xFF), byte(d.N[i]&0xFF))
	}
	return dst, nil
}
