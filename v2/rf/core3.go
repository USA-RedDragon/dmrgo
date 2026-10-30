//go:build !tinygo

package rf

func coreIQ3Options() options {
	o := coreExactOptions(tierOptions(Tier2))
	o.ExtLookback, o.CohSlotFit, o.CohWarm, o.CohRing = 16, true, true, 14
	o.CohTaps, o.CohCutHz, o.CohKaiser = coreCohTaps, coreCohCutHz, coreCohKaiser
	return o
}

func coreAudio3Options() options {
	o := tier2Options(discOptions())
	o.CohNoCross, o.CohNoNext, o.CohSymNoise, o.CohDataPasses = true, true, 8, 4
	return coreExactOptions(o)
}

func coreExactOptions(o options) options {
	o.CohExact, o.PhaseStates, o.LogMAP, o.CohBudget = true, 52, true, 0
	return o
}

const (
	coreCarsonHz   = deviationHz*3 + (1+rrcAlpha)*symbolRate/2
	coreCohPassHz  = coreCarsonHz + cohFreqSpan
	coreCohStopHz  = coreChannelHz - coreCarsonHz - cohFreqSpan
	coreCohCutHz   = (coreCohPassHz + coreCohStopHz) / 2
	coreCohAttenDB = 60
	coreCohKaiser  = 0.1102 * (coreCohAttenDB - 8.7)
	coreCohTaps    = 71
	coreChannelHz  = 12500
)
