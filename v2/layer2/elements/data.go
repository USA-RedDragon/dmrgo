package elements

type Data interface {
	// GetDataType returns the data type.
	GetDataType() DataType

	ToString() string
}
