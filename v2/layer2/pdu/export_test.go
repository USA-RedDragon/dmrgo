package pdu

import "github.com/USA-RedDragon/dmrgo/v2/bit"

func DecodeFLCGPSInfo(data [56]bit.Bit) FLCGPSInfo {
	var b [7]byte
	bit.AppendPack(b[:0], data[:])
	return decodeFLCGPSInfoBytes(b)
}

func DecodeCGAPContinuation(data [96]bit.Bit) CGAPContinuation {
	return decodeCGAPContinuationBytes(bit.Pack96(data))
}

func DecodeMVAPContinuation(data [96]bit.Bit) MVAPContinuation {
	return decodeMVAPContinuationBytes(bit.Pack96(data))
}

func DecodeBCAPContinuation(data [96]bit.Bit) BCAPContinuation {
	return decodeBCAPContinuationBytes(bit.Pack96(data))
}

func DecodeVNAPContinuation(data [96]bit.Bit) VNAPContinuation {
	return decodeVNAPContinuationBytes(bit.Pack96(data))
}
