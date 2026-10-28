//go:build !tinygo

package fsktx_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

var _ = hooks()

func hooks() bool {
	fsktx.RxAudio = rxAudio
	fsktx.RxIQ = rxIQ
	return true
}

func put(out []fsktx.RxBurst, bs []rf.Burst) []fsktx.RxBurst {
	for i := range bs {
		b := &bs[i]
		out = append(out, fsktx.RxBurst{Data: b.Data, Fixed: b.Fixed, CACH: b.CACH, FixedCACH: b.FixedCACH, HasFixed: b.HasFixed, HasCACH: b.HasCACH, Seconds: b.Seconds})
	}
	return out
}

func must(_ int, err error) {
	if err != nil {
		panic(err)
	}
}

func rxAudio(a []int16, hz, tier, chunk int) []fsktx.RxBurst {
	var out []fsktx.RxBurst
	r, err := rf.NewAudioReceiver(&rf.AudioRxConfig{Rate: hz, Tier: rf.Tier(tier), OnBursts: func(bs []rf.Burst) { out = put(out, bs) }})
	if err != nil {
		panic(err)
	}
	for k := 0; k < len(a); k += chunk {
		must(r.Write(a[k:min(k+chunk, len(a))]))
	}
	must(0, r.Flush())
	return out
}

func rxIQ(b []byte, rate, format, tier, chunk int) []fsktx.RxBurst {
	var out []fsktx.RxBurst
	f := rf.IQFormatCS8
	if format == fsktx.RxCF32 {
		f = rf.IQFormatCF32
	}
	r, err := rf.NewReceiver(&rf.RxConfig{Rate: rate, Format: f, Tier: rf.Tier(tier), OnBursts: func(bs []rf.Burst) { out = put(out, bs) }})
	if err != nil {
		panic(err)
	}
	for k := 0; k < len(b); k += chunk {
		must(r.Write(b[k:min(k+chunk, len(b))]))
	}
	must(0, r.Flush())
	return out
}
