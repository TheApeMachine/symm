package correlation

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous price-path correlation instrument. Each named
metric is an addressable Conn registered with the grid. Shared path state
runs once, then fans into those metric cells. Next admits an observation
into that graph and yields the export snapshot.
*/
type Ticker struct {
	*runtime.System
	Metrics map[string]*transport.Conn[*geometry.Coordinate]
	path    core.Primitive
	export  *store.Latest[string, float64]
}

func NewTicker(ctx context.Context, grid *store.Grid[*geometry.Coordinate]) *Ticker {
	export := store.NewLatest[string, float64]()
	price := [][]string{{"ticker", "data", "price"}}
	priceTime := [][]string{
		{"ticker", "data", "price"},
		{"ticker", "data", "timestamp"},
	}

	lastPrice := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewLastPrice()),
	)
	observationCount := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewObservationCount()),
	)
	signed := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewSigned()),
	)
	absolute := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewAbsoluteCorrelation()),
	)
	cohortSigned := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewCohortSigned()),
	)
	cohortAbsolute := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewCohortAbsolute()),
	)
	covariance := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewPairCovariance()),
	)
	referenceEnergy := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewReferenceEnergy()),
	)
	measuredEnergy := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewMeasuredEnergy()),
	)
	referenceEnergyRate := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewReferenceEnergyRate()),
	)
	measuredEnergyRate := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewMeasuredEnergyRate()),
	)
	overlapDensity := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewOverlapDensity()),
	)
	peerEnergy := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewPeerEnergy()),
	)
	measuredReturns := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewMeasuredReturns()),
	)
	referenceReturns := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewReferenceReturns()),
	)
	overlapCount := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewOverlapCount()),
	)
	sharedTime := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewSharedTime()),
	)
	pValue := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewPValue()),
	)
	standardError := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewStandardError()),
	)
	peerCount := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewPeerCount()),
	)
	effectivePeers := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewEffectivePeers()),
	)
	dispersion := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewDispersion()),
	)
	relative := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(
			nmcorrelation.NewEnergyPair(),
			arithmetic.NewDivide(),
		),
	)
	baseline := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewFisherBaseline()),
	)
	divergence := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewFisherDivergence()),
	)
	zscore := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(nmcorrelation.NewFisherZScore()),
	)
	correlationVelocity := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(
			nmcorrelation.NewFisherPoint(),
			temporal.NewVelocity(),
			temporal.NewRate(),
		),
	)
	energyBaseline := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(statistic.NewResidualBaseline()),
	)
	energyDivergence := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(statistic.NewResidualDivergence()),
	)
	energyZScore := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(statistic.NewResidualZScore()),
	)
	energyVelocity := transport.NewConn[*geometry.Coordinate](
		nomagique.NewNumber(
			nmcorrelation.NewEnergyPoint(),
			temporal.NewVelocity(),
			temporal.NewRate(),
		),
	)

	link := func(
		name string,
		conn *transport.Conn[*geometry.Coordinate],
		interests [][]string,
		extra ...core.Primitive,
	) {
		endpoint := sequence.Read[core.Connectable[*geometry.Coordinate]](
			nomagique.NewNumber(
				sequence.NewValues(interests),
				core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
					conn, core.Identify,
				),
				grid,
			).Next(nil),
		)
		branches := []core.Primitive{
			transport.NewIO[any](nil, nil),
			nomagique.NewNumber(
				store.NewWrite[string, float64](name),
				export,
				transport.NewDiscard(),
			),
		}
		branches = append(branches, extra...)
		endpoint.Connect(transport.NewFan(branches...))
	}

	link("last_price", lastPrice, price)
	link("observation_count", observationCount, price)
	link("signed_correlation", signed, priceTime,
		nomagique.NewNumber(
			nmcorrelation.NewFisherEstimator(),
			transport.NewFan(baseline, divergence, zscore),
			transport.NewDiscard(),
		),
	)
	link("absolute_correlation", absolute, priceTime)
	link("cohort_signed_correlation", cohortSigned, priceTime)
	link("cohort_absolute_correlation", cohortAbsolute, priceTime)
	link("covariance", covariance, priceTime)
	link("return_energy:reference", referenceEnergy, priceTime)
	link("return_energy:measured", measuredEnergy, priceTime)
	link("return_energy_rate:reference", referenceEnergyRate, priceTime)
	link("return_energy_rate:measured", measuredEnergyRate, priceTime)
	link("overlap_density", overlapDensity, priceTime)
	link("peer_return_energy_rate", peerEnergy, priceTime)
	link("supported_return_count:measured", measuredReturns, priceTime)
	link("supported_return_count:reference", referenceReturns, priceTime)
	link("overlap_pair_count", overlapCount, priceTime)
	link("shared_time", sharedTime, priceTime)
	link("correlation_p_value", pValue, priceTime)
	link("correlation_standard_error_fisher", standardError, priceTime)
	link("cohort_peer_count", peerCount, priceTime)
	link("cohort_effective_peer_count", effectivePeers, priceTime)
	link("cohort_correlation_dispersion", dispersion, priceTime)
	link("relative_return_energy", relative, priceTime,
		nomagique.NewNumber(
			equation.NewAdaptiveZScore(),
			transport.NewFan(energyBaseline, energyDivergence, energyZScore),
			transport.NewDiscard(),
		),
	)
	link("correlation_baseline", baseline, priceTime)
	link("correlation_divergence", divergence, priceTime)
	link("correlation_zscore", zscore, priceTime)
	link("correlation_velocity", correlationVelocity, priceTime)
	link("relative_return_energy_baseline", energyBaseline, priceTime)
	link("relative_return_energy_divergence", energyDivergence, priceTime)
	link("relative_return_energy_zscore", energyZScore, priceTime)
	link("relative_return_energy_velocity", energyVelocity, priceTime)

	ticker := &Ticker{
		Metrics: map[string]*transport.Conn[*geometry.Coordinate]{
			"last_price":                        lastPrice,
			"observation_count":                 observationCount,
			"signed_correlation":                signed,
			"absolute_correlation":              absolute,
			"cohort_signed_correlation":         cohortSigned,
			"cohort_absolute_correlation":       cohortAbsolute,
			"covariance":                        covariance,
			"return_energy:reference":           referenceEnergy,
			"return_energy:measured":            measuredEnergy,
			"return_energy_rate:reference":      referenceEnergyRate,
			"return_energy_rate:measured":       measuredEnergyRate,
			"overlap_density":                   overlapDensity,
			"peer_return_energy_rate":           peerEnergy,
			"supported_return_count:measured":   measuredReturns,
			"supported_return_count:reference":  referenceReturns,
			"overlap_pair_count":                overlapCount,
			"shared_time":                       sharedTime,
			"correlation_p_value":               pValue,
			"correlation_standard_error_fisher": standardError,
			"cohort_peer_count":                 peerCount,
			"cohort_effective_peer_count":       effectivePeers,
			"cohort_correlation_dispersion":     dispersion,
			"relative_return_energy":            relative,
			"correlation_baseline":              baseline,
			"correlation_divergence":            divergence,
			"correlation_zscore":                zscore,
			"correlation_velocity":              correlationVelocity,
			"relative_return_energy_baseline":   energyBaseline,
			"relative_return_energy_divergence": energyDivergence,
			"relative_return_energy_zscore":     energyZScore,
			"relative_return_energy_velocity":   energyVelocity,
		},
		path: transport.NewFan(
			lastPrice,
			nomagique.NewNumber(
				nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
				transport.NewFan(
					observationCount,
					signed,
					absolute,
					covariance,
					referenceEnergy,
					measuredEnergy,
					referenceEnergyRate,
					measuredEnergyRate,
					overlapDensity,
					measuredReturns,
					referenceReturns,
					overlapCount,
					sharedTime,
					pValue,
					standardError,
					relative,
					nomagique.NewNumber(
						nmcorrelation.NewAdmitted(),
						nmcorrelation.NewCohort(),
						transport.NewFan(
							cohortSigned,
							cohortAbsolute,
							peerEnergy,
							peerCount,
							effectivePeers,
							dispersion,
						),
					),
					correlationVelocity,
					energyVelocity,
				),
			),
		),
		export: export,
	}

	ticker.System = runtime.NewSystem(ctx, "correlation:ticker", ticker)
	ticker.Transition(runtime.READY)
	return ticker
}

/*
Next admits one price observation into the shared graph and yields the
export snapshot of metric values.
*/
func (ticker *Ticker) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if ticker.Status() != runtime.READY {
				errnie.Warn(ticker.Name() + ": Next called before READY; dropping event")

				if !yield(arriving) {
					return
				}

				continue
			}

			for range ticker.path.Next(sequence.NewOne(arriving).Next(nil)) {
			}

			command := store.LatestCommand[string, float64]{Read: true}
			snapshot := sequence.Read[map[string]float64](
				ticker.export.Next(sequence.NewOne(unsafe.Pointer(&command)).Next(nil)),
			)

			if !yield(unsafe.Pointer(&snapshot)) {
				return
			}
		}
	}
}
