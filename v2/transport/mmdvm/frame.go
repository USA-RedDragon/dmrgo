package mmdvm

import "bytes"

// FrameStart is the first octet of every frame.
const FrameStart = 0xE0

// MaxFrameLen is the length of the longest frame in octets: the long form
// counts 255 plus its second length octet.
const MaxFrameLen = 510

// Frame is one frame of the serial protocol: its command and its payload.
// Next points Payload into the buffer that it reads.
type Frame struct {
	// Command is what the frame carries.
	Command Command
	// Payload is the octets after the command.
	Payload []byte
}

// NextResult is the result of Next.
type NextResult uint8

// The NextResult values.
const (
	// NextFrame is a whole frame.
	NextFrame NextResult = iota
	// NextMore is the start of a frame that needs more octets.
	NextMore
	// NextSkip is octets that do not start a frame, to drop.
	NextSkip
)

// Next finds the frame at the start of p, as the firmware and MMDVMHost
// read the stream. It returns NextFrame and the length n of the frame;
// NextMore and n 0 when p ends inside a frame; or NextSkip and the
// number n of octets to drop, up to the next FrameStart, when p does not
// start with a frame. A frame starts with FrameStart and a length octet
// that counts the whole frame; a length of 0 is the long form, whose next
// octet plus 255 is the length, from 256 octets. A length too short for
// the header skips 1 octet. Next keeps no state and does not allocate.
func Next(p []byte) (f Frame, n int, r NextResult) {
	if len(p) == 0 {
		return Frame{}, 0, NextMore
	}
	if p[0] != FrameStart {
		i := bytes.IndexByte(p, FrameStart)
		if i < 0 {
			i = len(p)
		}
		return Frame{}, i, NextSkip
	}
	if len(p) < 2 {
		return Frame{}, 0, NextMore
	}
	total, head := int(p[1]), 3
	if total == 0 {
		if len(p) < 3 {
			return Frame{}, 0, NextMore
		}
		total, head = int(p[2])+255, 4
	}
	if total < head || head == 4 && total == 255 {
		return Frame{}, 1, NextSkip
	}
	if len(p) < total {
		return Frame{}, 0, NextMore
	}
	return Frame{Command: Command(p[head-1]), Payload: p[head:total:total]}, total, NextFrame
}

// AppendFrame appends a frame of c with payload to dst: the short form up
// to 255 octets in all, the long form above. It returns dst unchanged and an
// error for a frame longer than MaxFrameLen. It allocates only when dst has
// no room for len(payload)+4 more octets.
func AppendFrame(dst []byte, c Command, payload []byte) ([]byte, error) {
	switch n := len(payload) + 3; {
	case n <= 255:
		dst = append(dst, FrameStart, byte(n), byte(c))
	case n+1 <= MaxFrameLen:
		dst = append(dst, FrameStart, 0, byte(n+1-255), byte(c))
	default:
		return dst, errFrameLength
	}
	return append(dst, payload...), nil
}
