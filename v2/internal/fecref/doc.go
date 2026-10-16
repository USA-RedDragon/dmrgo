// Package fecref holds, in its subpackages, the FEC and CRC codecs that
// work on one bit.Bit per bit, as internal/fec and internal/crc did before
// they moved to packed bits. Only tests import them, to compare the packed
// codecs with them over random inputs.
package fecref
