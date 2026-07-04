package market

import (
	"math"
	"sort"

	"gonum.org/v1/gonum/stat"
)

type PeerCache struct {
	snapshot PeerSnapshot
	window   int
	version  int64
}

type PeerSnapshot struct {
	MarketReturns    []float64
	PeerCorrelations []float64
	PeerEnergies     []float64
	Series           []PeerSeries
}

type PeerSeries struct {
	Name    string
	Returns []float64
}

func NewPeerCache() *PeerCache {
	return &PeerCache{}
}

func (cache *PeerCache) Warm(crossSection *CrossSection, window int) {
	cache.Snapshot(crossSection, window)
}

func (cache *PeerCache) Snapshot(crossSection *CrossSection, window int) PeerSnapshot {
	if cache.window == window && cache.version == crossSection.version {
		return cache.snapshot
	}

	cache.snapshot = cache.build(crossSection, window)
	cache.window = window
	cache.version = crossSection.version

	return cache.snapshot
}

func (cache *PeerCache) MarketReturns(crossSection *CrossSection, window int) []float64 {
	return cache.Snapshot(crossSection, window).MarketReturns
}

func (cache *PeerCache) SymbolStats(crossSection *CrossSection, name string, window int) (
	correlation float64,
	energy float64,
	peerCorrelations []float64,
	peerEnergyMedian float64,
	ok bool,
) {
	returns := crossSection.SymbolReturns(name, window)
	if len(returns) < 2 {
		return 0, 0, nil, 0, false
	}

	effectiveWindow := min(len(returns), window)
	series := cache.series(crossSection, effectiveWindow)
	marketReturns := cache.marketReturnsExcluding(series, name)
	if len(marketReturns) < 2 {
		return 0, 0, nil, 0, false
	}

	returns = returns[len(returns)-len(marketReturns):]
	correlation = stat.Correlation(returns, marketReturns, nil)
	if math.IsNaN(correlation) || math.IsInf(correlation, 0) {
		return 0, 0, nil, 0, false
	}

	snapshot := cache.Snapshot(crossSection, effectiveWindow)
	return correlation,
		cache.medianAbsolute(returns),
		snapshot.PeerCorrelations,
		cache.medianPeerEnergy(snapshot.PeerEnergies),
		true
}

func (cache *PeerCache) build(crossSection *CrossSection, window int) PeerSnapshot {
	series := cache.series(crossSection, window)
	if len(series) < 2 {
		return PeerSnapshot{}
	}

	marketReturns := cache.marketReturns(series)
	peerCorrelations := make([]float64, 0, len(series))
	peerEnergies := make([]float64, 0, len(series))
	for _, peer := range series {
		peerCorrelation := stat.Correlation(peer.Returns, marketReturns, nil)
		if !math.IsNaN(peerCorrelation) && !math.IsInf(peerCorrelation, 0) {
			peerCorrelations = append(peerCorrelations, peerCorrelation)
		}
		peerEnergies = append(peerEnergies, cache.medianAbsolute(peer.Returns))
	}

	return PeerSnapshot{
		MarketReturns:    marketReturns,
		PeerCorrelations: peerCorrelations,
		PeerEnergies:     peerEnergies,
		Series:           series,
	}
}

func (cache *PeerCache) series(crossSection *CrossSection, window int) []PeerSeries {
	series := make([]PeerSeries, 0, len(crossSection.symbols))
	for _, symbol := range crossSection.symbols {
		returns := crossSection.SymbolReturns(symbol, window)
		if len(returns) == 0 {
			continue
		}

		series = append(series, PeerSeries{
			Name:    symbol,
			Returns: returns[len(returns)-min(len(returns), window):],
		})
	}

	if len(series) == 0 {
		return nil
	}

	commonWindow := len(series[0].Returns)
	for _, peer := range series[1:] {
		commonWindow = min(commonWindow, len(peer.Returns))
	}

	if commonWindow < 1 {
		return nil
	}

	for index := range series {
		returns := series[index].Returns
		series[index].Returns = returns[len(returns)-commonWindow:]
	}

	return series
}

func (cache *PeerCache) marketReturns(series []PeerSeries) []float64 {
	window := len(series[0].Returns)
	marketReturns := make([]float64, window)
	for index := range window {
		column := make([]float64, len(series))
		for peerIndex, peer := range series {
			column[peerIndex] = peer.Returns[index]
		}
		sort.Float64s(column)
		marketReturns[index] = stat.Quantile(0.5, stat.LinInterp, column, nil)
	}

	return marketReturns
}

func (cache *PeerCache) marketReturnsExcluding(series []PeerSeries, name string) []float64 {
	peers := make([]PeerSeries, 0, len(series))
	for _, peer := range series {
		if peer.Name != name {
			peers = append(peers, peer)
		}
	}

	if len(peers) == 0 {
		return nil
	}

	return cache.marketReturns(peers)
}

func (cache *PeerCache) medianPeerEnergy(energies []float64) float64 {
	if len(energies) == 0 {
		return 0
	}

	sorted := append([]float64(nil), energies...)
	sort.Float64s(sorted)

	return stat.Quantile(0.5, stat.LinInterp, sorted, nil)
}

func (cache *PeerCache) medianAbsolute(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	absValues := make([]float64, len(values))
	for index, value := range values {
		absValues[index] = math.Abs(value)
	}
	sort.Float64s(absValues)

	return stat.Quantile(0.5, stat.LinInterp, absValues, nil)
}
