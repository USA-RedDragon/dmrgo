package rf

import "github.com/USA-RedDragon/dmrgo/v2/internal/rftx"

const burstSeconds = 0.0275

func mustNew(cfg Config) *Receiver {
	r, err := New(&cfg)
	if err != nil {
		panic(err)
	}
	return r
}

func testConfig() rftx.SeqConfig {
	return rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
}

func must(b []Burst, err error) []Burst {
	if err != nil {
		panic(err)
	}
	return b
}
