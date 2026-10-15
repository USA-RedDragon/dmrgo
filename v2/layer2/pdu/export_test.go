package pdu

import "github.com/USA-RedDragon/dmrgo/v2/bit"

func DecodeFLCGPSInfo(data [56]bit.Bit) FLCGPSInfo { return decodeFLCGPSInfo(data) }

func DecodeCGAPContinuation(data [96]bit.Bit) CGAPContinuation { return decodeCGAPContinuation(data) }

func DecodeMVAPContinuation(data [96]bit.Bit) MVAPContinuation { return decodeMVAPContinuation(data) }

func DecodeBCAPContinuation(data [96]bit.Bit) BCAPContinuation { return decodeBCAPContinuation(data) }

func DecodeVNAPContinuation(data [96]bit.Bit) VNAPContinuation { return decodeVNAPContinuation(data) }
