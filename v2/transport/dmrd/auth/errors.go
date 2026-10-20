package auth

import "errors"

var (
	errDialect = errors.New("auth: Token takes DialectMMDVM or DialectHBRP")
	errPacket  = errors.New("auth: not an OpenBridge packet")
	errKey     = errors.New("auth: a BLAKE2b key is at most 64 octets")
)
