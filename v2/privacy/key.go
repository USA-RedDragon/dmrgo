package privacy

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

// Algorithm is the algorithm ID octet of a PI header with FID 0x10.
type Algorithm uint8

// The Algorithm values that a Key can have. Basic Privacy has no algorithm
// ID on air; its keys report AlgorithmBasic.
const (
	// AlgorithmBasic is Basic Privacy, a fixed pattern from a 16-bit key.
	// It is 0, the value of no algorithm, because no PI header carries it.
	AlgorithmBasic Algorithm = 0x00
	// AlgorithmARC4 is ARC4 (RC4) with a 40-bit key, also called Enhanced
	// Privacy.
	AlgorithmARC4 Algorithm = 0x21
	// AlgorithmDES is DES in OFB mode. The package detects it but has no
	// cipher for it.
	AlgorithmDES Algorithm = 0x22
	// AlgorithmAES128 is AES-128 in OFB mode.
	AlgorithmAES128 Algorithm = 0x24
	// AlgorithmAES256 is AES-256 in OFB mode.
	AlgorithmAES256 Algorithm = 0x25
)

// String returns the name of the algorithm. It allocates only for a value
// without a name.
func (a Algorithm) String() string {
	switch a {
	case AlgorithmBasic:
		return "Basic"
	case AlgorithmARC4:
		return "ARC4"
	case AlgorithmDES:
		return "DES"
	case AlgorithmAES128:
		return "AES-128"
	case AlgorithmAES256:
		return "AES-256"
	}
	return fmt.Sprintf("Algorithm(0x%02X)", uint8(a))
}

// Key is one privacy key with its algorithm and key ID. Make it with
// NewARC4Key, NewAESKey or NewBasicKey. A Key does not change after it is
// made, so Receivers and Transmitters can share it.
type Key struct {
	alg   Algorithm
	id    uint8
	arc4  [5]byte
	basic [6]byte
	block cipher.Block
}

// NewARC4Key returns the 40-bit ARC4 key with the key ID id. It allocates
// the Key.
func NewARC4Key(id uint8, key [5]byte) *Key {
	return &Key{alg: AlgorithmARC4, id: id, arc4: key}
}

// NewAESKey returns the AES key with the key ID id: AES-128 for 16 octets
// and AES-256 for 32. It returns a *dmrgo.ConfigError for another length.
// It copies key and allocates the Key and its AES block cipher.
func NewAESKey(id uint8, key []byte) (*Key, error) {
	var alg Algorithm
	switch len(key) {
	case 16:
		alg = AlgorithmAES128
	case 32:
		alg = AlgorithmAES256
	default:
		return nil, errAESKey
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, errAESKey
	}
	return &Key{alg: alg, id: id, block: b}, nil
}

// NewBasicKey returns the Basic Privacy key with the 16-bit value key. Its
// key ID is 0. The value is not the key number of 1 to 255 that a radio
// shows. It allocates the Key.
func NewBasicKey(key uint16) *Key {
	hi, lo := byte(key>>8), byte(key&0xFF)
	return &Key{alg: AlgorithmBasic, basic: [6]byte{hi, lo & 0x0F, hi, lo, hi, lo}}
}

// Algorithm returns the algorithm of the key, or AlgorithmBasic for a nil
// Key.
func (k *Key) Algorithm() Algorithm {
	if k == nil {
		return AlgorithmBasic
	}
	return k.alg
}

// ID returns the key ID of the key, or 0 for a nil Key.
func (k *Key) ID() uint8 {
	if k == nil {
		return 0
	}
	return k.id
}
