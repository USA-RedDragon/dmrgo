package layer2

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// Data is the PDU that a data or control burst carries.
type Data interface {
	// GetDataType returns the data type.
	GetDataType() enums.DataType

	ToString() string
}
