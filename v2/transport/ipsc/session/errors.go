package session

import "github.com/USA-RedDragon/dmrgo/v2"

const errType = "ipsc/session"

var (
	errTimeout   error = &dmrgo.ConfigError{Type: errType, Field: "Timeout"}
	errAlive     error = &dmrgo.ConfigError{Type: errType, Field: "Alive"}
	errMaxMissed error = &dmrgo.ConfigError{Type: errType, Field: "MaxMissed"}
	errMaxPeers  error = &dmrgo.ConfigError{Type: errType, Field: "MaxPeers"}
)
