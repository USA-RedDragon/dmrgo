package session_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/session"
)

func ipscKey() ([20]byte, error) {
	return auth.ParseKey("0123456789abcdef")
}

func kinds(out []session.Output) []session.OutputKind {
	k := make([]session.OutputKind, len(out))
	for i, o := range out {
		k[i] = o.Kind
	}
	return k
}

func sends(out []session.Output) [][]byte {
	var s [][]byte
	for _, o := range out {
		if o.Kind == session.OutputSend {
			s = append(s, append([]byte(nil), o.Packet...))
		}
	}
	return s
}
