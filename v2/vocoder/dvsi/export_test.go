package dvsi

import "time"

type funcClock func() time.Time

func (f funcClock) Now() time.Time { return f() }

func SetClock(cfg *Config, now func() time.Time) {
	cfg.clock = funcClock(now)
}
