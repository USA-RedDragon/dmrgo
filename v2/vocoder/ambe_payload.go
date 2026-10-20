package vocoder

import "github.com/USA-RedDragon/dmrgo/v2/internal/bits"

const (
	ambeBad2Bit = 49
	ambeBad3Bit = 99
)

// PackAMBEVoice packs the three frames of a voice burst into the 19-byte
// IPSC AMBE payload. The bits that carry no frame bits are 0. It returns
// the error of EncodeAMBE72 when a padding bit of a frame is set. It does
// not allocate.
func PackAMBEVoice(frames [3]AMBEFrame) ([19]byte, error) {
	var data [19]byte
	for k := range frames {
		if frames[k][6]&0x7F != 0 {
			return [19]byte{}, errAMBEFrameRange
		}
		bits.Copy(data[:], k*50, frames[k][:], 0, 49)
	}
	return data, nil
}

// UnpackAMBEVoice is the reverse of PackAMBEVoice. It ignores the bits that
// carry no frame bits. It does not allocate.
func UnpackAMBEVoice(data [19]byte) [3]AMBEFrame {
	var frames [3]AMBEFrame
	for k := range frames {
		bits.Copy(frames[k][:], 0, data[:], k*50, 49)
	}
	return frames
}

// PackAMBEVoiceFlags is PackAMBEVoice that also sets bit 49 when bad[1] is
// true and bit 99 when bad[2] is true, to flag frames 2 and 3 as bad. It
// ignores bad[0]: the bad flag of frame 1 is in the IPSC packet header, not
// in the AMBE payload. It returns the errors of PackAMBEVoice. It does not
// allocate.
func PackAMBEVoiceFlags(frames [3]AMBEFrame, bad [3]bool) ([19]byte, error) {
	data, err := PackAMBEVoice(frames)
	if err != nil {
		return data, err
	}
	if bad[1] {
		data[ambeBad2Bit/8] |= 0x80 >> (ambeBad2Bit % 8)
	}
	if bad[2] {
		data[ambeBad3Bit/8] |= 0x80 >> (ambeBad3Bit % 8)
	}
	return data, nil
}

// UnpackAMBEVoiceFlags is the reverse of PackAMBEVoiceFlags. bad[1] and
// bad[2] report bits 49 and 99. bad[0] is always false, because the bad flag
// of frame 1 is in the IPSC packet header. It does not allocate.
func UnpackAMBEVoiceFlags(data [19]byte) (frames [3]AMBEFrame, bad [3]bool) {
	bad[1] = data[ambeBad2Bit/8]&(0x80>>(ambeBad2Bit%8)) != 0
	bad[2] = data[ambeBad3Bit/8]&(0x80>>(ambeBad3Bit%8)) != 0
	return UnpackAMBEVoice(data), bad
}
