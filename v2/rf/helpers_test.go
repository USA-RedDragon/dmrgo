package rf

import "github.com/USA-RedDragon/dmrgo/v2/internal/rftx"

// burstSeconds is the length of a burst without its CACH.
const burstSeconds = 0.0275

// mustNew is New for a configuration that a test knows is valid.
func mustNew(cfg Config) *Receiver {
	r, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return r
}

// testConfig returns the sequence settings of the loopback tests.
func testConfig() rftx.SeqConfig {
	return rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
}
