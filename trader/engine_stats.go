package trader

/*
EngineStats exposes live market counters for engine_pulse telemetry.
*/
type EngineStats interface {
	TickerReadyCount() int
	SymbolTotal() int
	FluidSampledCount() int
	FluidWarmingCount() int
}

type engineStatsFunc struct {
	tickerReady func() int
	symbolTotal func() int
	fluidSample func() int
	fluidWarm   func() int
}

/*
NewEngineStats builds an EngineStats from probe callbacks.
*/
func NewEngineStats(
	tickerReady func() int,
	symbolTotal func() int,
	fluidSample func() int,
	fluidWarm func() int,
) EngineStats {
	return engineStatsFunc{
		tickerReady: tickerReady,
		symbolTotal: symbolTotal,
		fluidSample: fluidSample,
		fluidWarm:   fluidWarm,
	}
}

func (stats engineStatsFunc) TickerReadyCount() int {
	if stats.tickerReady == nil {
		return 0
	}

	return stats.tickerReady()
}

func (stats engineStatsFunc) SymbolTotal() int {
	if stats.symbolTotal == nil {
		return 0
	}

	return stats.symbolTotal()
}

func (stats engineStatsFunc) FluidSampledCount() int {
	if stats.fluidSample == nil {
		return 0
	}

	return stats.fluidSample()
}

func (stats engineStatsFunc) FluidWarmingCount() int {
	if stats.fluidWarm == nil {
		return 0
	}

	return stats.fluidWarm()
}
