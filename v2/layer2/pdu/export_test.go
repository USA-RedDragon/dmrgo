package pdu

func DecodeFLCGPSInfo(data [7]byte) FLCGPSInfo {
	return decodeFLCGPSInfo(data)
}

func DecodeCGAPContinuation(data [12]byte) CGAPContinuation {
	return decodeCGAPContinuation(data)
}

func DecodeMVAPContinuation(data [12]byte) MVAPContinuation {
	return decodeMVAPContinuation(data)
}

func DecodeBCAPContinuation(data [12]byte) BCAPContinuation {
	return decodeBCAPContinuation(data)
}

func DecodeVNAPContinuation(data [12]byte) VNAPContinuation {
	return decodeVNAPContinuation(data)
}
