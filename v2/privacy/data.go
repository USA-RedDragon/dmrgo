package privacy

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const dataEncrypted = 0x1

// FromProprietaryHeader returns the Info of the privacy header h of a data
// packet: a P_HEAD with FID 0x10 whose first manufacturer octet holds the
// algorithm in bits 1 to 3 and 1 in its low 4 bits, followed by the key ID,
// 2 octets and the 32-bit MI. The algorithm 1, 2, 4 or 5 maps to the PI
// header IDs AlgorithmARC4, AlgorithmDES, AlgorithmAES128 and
// AlgorithmAES256, and 0 to AlgorithmBasic. It returns the zero Info for a
// nil h and for another P_HEAD. ETSI TS 102 361-1 clause 9.2.9 defines only
// the P_HEAD; the layout is that of public captures.
func FromProprietaryHeader(h *pdu.ProprietaryHeader) Info {
	if h == nil || h.MFID != enums.FIDMotorola || h.ManufacturerData[0]&0x0F != dataEncrypted {
		return Info{}
	}
	alg := Algorithm(h.ManufacturerData[0] >> 4 & 0x07)
	if alg != AlgorithmBasic {
		alg |= 0x20
	}
	return Info{
		Encrypted: true,
		Header:    true,
		MFID:      h.MFID,
		Algorithm: alg,
		KeyID:     h.ManufacturerData[1],
		MI:        binary.BigEndian.Uint32(h.ManufacturerData[4:8]),
	}
}

// Data removes the privacy of the data packet m in place and returns its
// Info. When the P_HEAD of m is a privacy header (see FromProprietaryHeader)
// with ARC4 and the Receiver holds its key, it XORs m.Payload with the RC4
// keystream of the key octets and the 4 MI octets after the first 256
// octets, and the Info reports Decrypting; the pad octets and the message
// CRC are not encrypted, so the CRC that the data.Receiver checked is that
// of the encrypted payload. A packet with another algorithm or without its
// key stays as it is. Data does not change the voice call of the Receiver.
// A nil m returns the zero Info.
func (r *Receiver) Data(m *data.Message) Info {
	if r == nil || m == nil {
		return Info{}
	}
	info := FromProprietaryHeader(m.Proprietary)
	if !info.Header || info.Algorithm != AlgorithmARC4 {
		return info
	}
	k := r.lookup(&info)
	if k == nil {
		return info
	}
	copy(r.dataKey[:5], k.arc4[:])
	binary.BigEndian.PutUint32(r.dataKey[5:], info.MI)
	r.dataRC.init(r.dataKey[:])
	for range arc4Drop {
		r.dataRC.next()
	}
	for i := range m.Payload {
		m.Payload[i] ^= r.dataRC.next()
	}
	r.dataRC = arc4{}
	clear(r.dataKey[:])
	info.Decrypting = true
	return info
}
