//go:build !tinygo

package rf

import (
	"math"
	"strconv"
)

var rxMatrixLimits = []mxLimit{
	{
		match: func(r mxRow, _ mxRx) bool {
			off, _ := strconv.ParseFloat(r.val(mxOffset), 64)
			return r.val(mxFormat) == "cs16-9600" && math.Abs(off) >= 2500
		},
		checks: "all",
		reason: "low-rate I/Q at 9.6 kHz holds 4.8 kHz on each side; 2.5 kHz or more off, 9.8 dB or more of the signal power is outside the channel filter of the radio chip",
		doc:    "docs/rx.md, low-rate I/Q tier 0 (owner accepted 2026-10-29)",
	},
}

var rxMatrixBugs = map[string]string{}

var rxMatrixXFail = map[string]mxXFail{
	"agc.none.-4000.ms.voice.-500ppm.high.cs16-2048k.inf/t0":          {checks: "bits"},
	"gap.cw2s.+700.ms2.pi.0ppm.low.cs16-48k.inf.dc.step10dB/t0":       {checks: "missing"},
	"gap.cw2s.+700.ms2.pi.0ppm.low.cs16-48k.inf.dc.step10dB/t1":       {checks: "missing"},
	"gap.cw2s.+700.ms2.pi.0ppm.low.cs16-48k.inf.dc.step10dB/t2":       {checks: "missing"},
	"gap.cw500ms.+4000.bs.csbk.0ppm.low.a48000.inf.step10dB/t0":       {checks: "cach"},
	"gap.cw500ms.+4000.bs.csbk.0ppm.low.a48000.inf.step10dB/t1":       {checks: "cach"},
	"gap.cw500ms.+4000.bs.csbk.0ppm.low.a48000.inf.step10dB/t2":       {checks: "cach"},
	"gap.cw500ms.+4000.bs.csbk.0ppm.low.a48000.inf.step10dB/t3":       {checks: "cach"},
	"gap.none.+3000.ms.data34.+500ppm.nom.cs16-2048k.40dB/t0":         {checks: "missing"},
	"gap.none.+4000.ms.data1.0ppm.low.cs16-2048k.inf/t0":              {checks: "missing"},
	"gap.none.-2500.bs.data1.-500ppm.nom.a48000.40dB/t0":              {checks: "cach"},
	"gap.none.-2500.bs.data1.-500ppm.nom.a48000.40dB/t1":              {checks: "cach"},
	"gap.none.-2500.bs.data1.-500ppm.nom.a48000.40dB/t2":              {checks: "cach"},
	"gap.none.-2500.bs.data1.-500ppm.nom.a48000.40dB/t3":              {checks: "cach"},
	"mid.none.+4000.ms2.voice.+500ppm.nom.cs16-48k.inf.dc/t0":         {checks: "missing"},
	"mid.none.+4000.ms2.voice.+500ppm.nom.cs16-48k.inf.dc/t1":         {checks: "missing"},
	"mid.none.+4000.ms2.voice.+500ppm.nom.cs16-48k.inf.dc/t2":         {checks: "missing"},
	"mid.none.-4000.ms.voice.-500ppm.high.cu8-48k.40dB/t0":            {checks: "missing"},
	"mid.none.-4000.ms.voice.-500ppm.high.cu8-48k.40dB/t1":            {checks: "missing"},
	"mid.none.-4000.ms.voice.-500ppm.high.cu8-48k.40dB/t2":            {checks: "missing"},
	"mid.none.-700.ms.voice.0ppm.high.a9600.inf/t0":                   {checks: "missing"},
	"mid.none.-700.ms.voice.0ppm.high.a9600.inf/t1":                   {checks: "missing"},
	"mid.none.-700.ms.voice.0ppm.high.a9600.inf/t2":                   {checks: "missing"},
	"mid.none.-700.ms.voice.0ppm.high.a9600.inf/t3":                   {checks: "missing"},
	"s0.cw500ms.-2500.ms2.data34.+500ppm.high.a24000.inf.step10dB/t0": {checks: "missing"},
	"s0.cw500ms.-2500.ms2.data34.+500ppm.high.a24000.inf.step10dB/t1": {checks: "missing"},
	"s0.cw500ms.-2500.ms2.data34.+500ppm.high.a24000.inf.step10dB/t2": {checks: "missing"},
	"s0.cw500ms.-2500.ms2.data34.+500ppm.high.a24000.inf.step10dB/t3": {checks: "missing"},
	"s0.noise.-4000.bs.data12.-500ppm.low.a9600.40dB.step10dB/t0":     {checks: "cach"},
	"s0.noise.-4000.bs.data12.-500ppm.low.a9600.40dB.step10dB/t1":     {checks: "cach"},
	"s0.noise.-4000.bs.data12.-500ppm.low.a9600.40dB.step10dB/t2":     {checks: "cach"},
	"s0.noise.-4000.bs.data12.-500ppm.low.a9600.40dB.step10dB/t3":     {checks: "cach"},
	"s0.none.+3000.ms2.rc.-500ppm.low.cu8-2048k.40dB.dc/t0":           {checks: "missing"},
	"s0.none.+4000.ms.rc.+500ppm.high.cu8-48k.40dB/t0":                {checks: "missing"},
}
