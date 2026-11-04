package rf

import "github.com/USA-RedDragon/dmrgo/v2/internal/rftx"

const burstSeconds = 0.0275

func testConfig() rftx.SeqConfig {
	return rftx.SeqConfig{ColorCode: 1, Source: 3009101, Dest: 9990, BS: 990911, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
}

func fmMid(f []float64, k int) float64 {
	if k == 0 {
		return f[0]
	}
	return (f[k-1] + f[k]) / 2
}

func must(b []Burst, err error) []Burst {
	if err != nil {
		panic(err)
	}
	return b
}
