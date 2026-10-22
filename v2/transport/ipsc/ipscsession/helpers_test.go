package ipscsession_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func ipscKey() ([20]byte, error) {
	return auth.ParseKey("0123456789abcdef")
}

func kinds(out []ipscsession.Output) []ipscsession.OutputKind {
	k := make([]ipscsession.OutputKind, len(out))
	for i, o := range out {
		k[i] = o.Kind
	}
	return k
}

func sends(out []ipscsession.Output) [][]byte {
	var s [][]byte
	for _, o := range out {
		if o.Kind == ipscsession.OutputSend {
			s = append(s, append([]byte(nil), o.Packet...))
		}
	}
	return s
}
