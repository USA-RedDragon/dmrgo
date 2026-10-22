package dmrdauth

import "github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"

type wrapError struct {
	msg string
	err error
}

func (e *wrapError) Error() string {
	return e.msg
}

func (e *wrapError) Unwrap() error {
	return e.err
}

var (
	errDialect error = &wrapError{"dmrdauth: Token takes DialectMMDVM or DialectHBRP", dmrd.ErrCommand}
	errPacket  error = &wrapError{"dmrdauth: not an OpenBridge packet", dmrd.ErrCommand}
)
