package packet

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
)

func BlockOctets(rate enums.DataType, confirmed bool) (int, error) {
	return blockOctets(rate, confirmed)
}

func Layout(n, per int) (blocks, pad int) { return layout(n, per) }

func MessageCRC(data []byte) uint32 { return crc.CRC32(data) }

func AppendMessageCRC(data []byte) []byte { return appendMessageCRC(data) }

func CheckMessageCRC(data []byte) bool { return checkMessageCRC(data) }
