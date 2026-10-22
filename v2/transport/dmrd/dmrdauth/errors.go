package dmrdauth

import "errors"

var (
	errDialect = errors.New("dmrdauth: Token takes DialectMMDVM or DialectHBRP")
	errPacket  = errors.New("dmrdauth: not an OpenBridge packet")
)
