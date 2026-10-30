//go:build !tinygo

package rf

func coreIQ3Options() options {
	o := coreExactOptions(tierOptions(Tier2))
	o.ExtLookback, o.CohSlotFit, o.CohWarm, o.CohRing = 16, true, true, 14
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
