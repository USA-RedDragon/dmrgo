package privacy

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2"
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
	xorData(&r.dataRC, &r.dataKey, k, info.MI, m.Payload)
	info.Decrypting = true
	return info
}

func xorData(rc *arc4, key *[9]byte, k *Key, mi uint32, p []byte) {
	copy(key[:5], k.arc4[:])
	binary.BigEndian.PutUint32(key[5:], mi)
	rc.init(key[:])
	for range arc4Drop {
		rc.next()
	}
	for i := range p {
		p[i] ^= rc.next()
	}
	*rc = arc4{}
	clear(key[:])
}

// Data applies ARC4 privacy to payload in place, the user data of a data
// packet without the pad octets and the message CRC, with the message
// indicator mi, and returns the privacy header of the packet: a P_HEAD
// with FID 0x10 and the SAP sap of the user data that holds the algorithm,
// the key ID and mi, the layout that FromProprietaryHeader reads. It XORs
// payload with the RC4 keystream of the key octets and the 4 MI octets
// after the first 256 octets, which Receiver.Data removes. For
// data.Sender.Send, put the header in data.Message.Proprietary and set the
// SAP of the U_HEAD or C_HEAD to enums.SAPProprietaryPacketData, as radios
// do. Use a new random mi for each packet, such as 4 octets from
// crypto/rand: a repeated mi repeats the keystream. Data does not change
// the voice call of the Transmitter. It returns ErrDataAlgorithm for a key
// other than ARC4, as no public capture with a key proves the data framing
// of the others, and a *dmrgo.ConfigError for a sap over 15, and then
// leaves payload as it is.
func (t *Transmitter) Data(mi uint32, sap enums.SAP, payload []byte) (pdu.ProprietaryHeader, error) {
	if t == nil {
		return pdu.ProprietaryHeader{}, dmrgo.ErrNilReceiver
	}
	if t.key == nil || t.key.alg != AlgorithmARC4 {
		return pdu.ProprietaryHeader{}, ErrDataAlgorithm
	}
	if sap > 15 {
		return pdu.ProprietaryHeader{}, errSAP
	}
	xorData(&t.dataRC, &t.dataKey, t.key, mi, payload)
	h := pdu.ProprietaryHeader{SAP: sap, MFID: enums.FIDMotorola}
	h.ManufacturerData[0] = byte(t.key.alg&0x07)<<4 | dataEncrypted
	h.ManufacturerData[1] = t.key.id
	binary.BigEndian.PutUint32(h.ManufacturerData[4:], mi)
	return h, nil
}
