package dmrdsession

import "github.com/USA-RedDragon/dmrgo/v2"

const errType = "dmrdsession"

var (
	errDialect      error = &dmrgo.ConfigError{Type: errType, Field: "Dialect"}
	errConfig       error = &dmrgo.ConfigError{Type: errType, Field: nameConfig}
	errOptions      error = &dmrgo.ConfigError{Type: errType, Field: nameOptions}
	errRetry        error = &dmrgo.ConfigError{Type: errType, Field: "Retry"}
	errTimeout      error = &dmrgo.ConfigError{Type: errType, Field: "Timeout"}
	errBackoff      error = &dmrgo.ConfigError{Type: errType, Field: "Backoff"}
	errLoginTimeout error = &dmrgo.ConfigError{Type: errType, Field: "LoginTimeout"}
	errKeepalive    error = &dmrgo.ConfigError{Type: errType, Field: "Keepalive"}
)
