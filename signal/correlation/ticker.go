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
Ticker registers correlation metric Conns on the grid. It does not route
market data or compute. Next reads already-retained observations from the grid.
*/
type Ticker struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTicker(
	ctx context.Context,
	grid *store.Grid[*geometry.Coordinate],
	measured, reference string,
	cohort ...string,
) *Ticker {
	symbolLast := [][]string{
		{"ticker", "data", "symbol"},
		{"ticker", "data", "last"},
	}
	symbolLastTime := [][]string{
		{"ticker", "data", "symbol"},
		{"ticker", "data", "last"},
		{"ticker", "data", "timestamp"},
	}

	register := func(conn *transport.Conn[*geometry.Coordinate], interests [][]string) {
		sequence.Read[core.Connectable[*geometry.Coordinate]](
			nomagique.NewNumber(
				sequence.NewValues(interests),
				core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
					conn, core.Identify,
				),
				grid,
			).Next(nil),
		)
	}

	lastHold := store.NewKeyed[float64]()
	lastPrice := store.NewStamp(
		nmcorrelation.NewLastPrice(),
		func(observation *nmcorrelation.PriceObservation) string { return observation.Symbol },
	)
	lastStages := []core.Primitive{nmcorrelation.NewTick(), lastPrice, lastHold}

	if measured != "" {
		lastStages = []core.Primitive{
			nmcorrelation.NewTick(),
			nmcorrelation.NewMatch(measured),
			lastPrice,
			lastHold,
		}
	}

	lastPrice := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(lastStages...))
	register(lastPrice, symbolLast)

	if measured != "" && reference != "" {
		countHold := store.NewKeyed[float64]()
		signedHold := store.NewKeyed[float64]()
		absoluteHold := store.NewKeyed[float64]()
		covarianceHold := store.NewKeyed[float64]()
		refEnergyHold := store.NewKeyed[float64]()
		measEnergyHold := store.NewKeyed[float64]()
		refRateHold := store.NewKeyed[float64]()
		measRateHold := store.NewKeyed[float64]()
		densityHold := store.NewKeyed[float64]()
		measReturnsHold := store.NewKeyed[float64]()
		refReturnsHold := store.NewKeyed[float64]()
		overlapHold := store.NewKeyed[float64]()
		sharedHold := store.NewKeyed[float64]()
		pValueHold := store.NewKeyed[float64]()
		errorHold := store.NewKeyed[float64]()
		relativeHold := store.NewKeyed[float64]()
		baselineHold := store.NewKeyed[float64]()
		divergenceHold := store.NewKeyed[float64]()
		zscoreHold := store.NewKeyed[float64]()
		velocityHold := store.NewKeyed[float64]()
		energyBaseHold := store.NewKeyed[float64]()
		energyDivHold := store.NewKeyed[float64]()
		energyZHold := store.NewKeyed[float64]()
		energyVelHold := store.NewKeyed[float64]()
		fixed := store.NewFixed(measured)

		count := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewObservationCount(measured), fixed, fixed, countHold),
		)
		absolute := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewAbsoluteCorrelation(), fixed, absoluteHold),
		)
		covariance := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewPairCovariance(), fixed, covarianceHold),
		)
		refEnergy := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewReferenceEnergy(), fixed, refEnergyHold),
		)
		measEnergy := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewMeasuredEnergy(), fixed, measEnergyHold),
		)
		refRate := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewReferenceEnergyRate(), fixed, refRateHold),
		)
		measRate := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewMeasuredEnergyRate(), fixed, measRateHold),
		)
		density := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewOverlapDensity(), fixed, densityHold),
		)
		measReturns := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewMeasuredReturns(), fixed, measReturnsHold),
		)
		refReturns := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewReferenceReturns(), fixed, refReturnsHold),
		)
		overlap := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewOverlapCount(), fixed, overlapHold),
		)
		shared := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewSharedTime(), fixed, sharedHold),
		)
		pValue := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewPValue(), fixed, pValueHold),
		)
		stdError := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewStandardError(), fixed, errorHold),
		)
		relative := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewEnergyPair(), arithmetic.NewDivide(), fixed, relativeHold),
		)
		baseline := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewFisherBaseline(), fixed, baselineHold),
		)
		divergence := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewFisherDivergence(), fixed, divergenceHold),
		)
		zscore := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewFisherZScore(), fixed, zscoreHold),
		)
		velocity := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewFisherPoint(), temporal.NewVelocity(), temporal.NewRate(), fixed, velocityHold),
		)
		energyBase := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualBaseline(), fixed, energyBaseHold),
		)
		energyDiv := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualDivergence(), fixed, energyDivHold),
		)
		energyZ := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualZScore(), fixed, energyZHold),
		)
		energyVel := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewEnergyPoint(), temporal.NewVelocity(), temporal.NewRate(), fixed, energyVelHold),
		)

		pairBranches := []core.Primitive{
			transport.NewIO[any](nil, nil),
			nomagique.NewNumber(nmcorrelation.NewObservationCount(measured), fixed, countHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewAbsoluteCorrelation(), fixed, absoluteHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewPairCovariance(), fixed, covarianceHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewReferenceEnergy(), fixed, refEnergyHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewMeasuredEnergy(), fixed, measEnergyHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewReferenceEnergyRate(), fixed, refRateHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewMeasuredEnergyRate(), fixed, measRateHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewOverlapDensity(), fixed, densityHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewMeasuredReturns(), fixed, measReturnsHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewReferenceReturns(), fixed, refReturnsHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewOverlapCount(), fixed, overlapHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewSharedTime(), fixed, sharedHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewPValue(), fixed, pValueHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewStandardError(), fixed, errorHold, transport.NewDiscard()),
			nomagique.NewNumber(
				nmcorrelation.NewEnergyPair(),
				arithmetic.NewDivide(),
				fixed, relativeHold,
				nomagique.NewNumber(
					equation.NewAdaptiveZScore(),
					transport.NewFan(
						nomagique.NewNumber(statistic.NewResidualBaseline(), fixed, energyBaseHold, transport.NewDiscard()),
						nomagique.NewNumber(statistic.NewResidualDivergence(), fixed, energyDivHold, transport.NewDiscard()),
						nomagique.NewNumber(statistic.NewResidualZScore(), fixed, energyZHold, transport.NewDiscard()),
					),
					transport.NewDiscard(),
				),
				transport.NewDiscard(),
			),
			nomagique.NewNumber(
				nmcorrelation.NewFisherPoint(),
				temporal.NewVelocity(),
				temporal.NewRate(),
				fixed, velocityHold,
				transport.NewDiscard(),
			),
			nomagique.NewNumber(
				nmcorrelation.NewEnergyPoint(),
				temporal.NewVelocity(),
				temporal.NewRate(),
				fixed, energyVelHold,
				transport.NewDiscard(),
			),
		}

		if len(cohort) > 0 {
			peerHold := store.NewKeyed[float64]()
			cohortSignedHold := store.NewKeyed[float64]()
			cohortAbsHold := store.NewKeyed[float64]()
			peerCountHold := store.NewKeyed[float64]()
			effectiveHold := store.NewKeyed[float64]()
			dispersionHold := store.NewKeyed[float64]()

			peerEnergy := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewPeerEnergy(), fixed, peerHold),
			)
			cohortSigned := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewCohortSigned(), fixed, cohortSignedHold),
			)
			cohortAbs := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewCohortAbsolute(), fixed, cohortAbsHold),
			)
			peerCount := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewPeerCount(), fixed, peerCountHold),
			)
			effective := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewEffectivePeers(), fixed, effectiveHold),
			)
			dispersion := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewDispersion(), fixed, dispersionHold),
			)

			pairBranches = append(pairBranches, nomagique.NewNumber(
				nmcorrelation.NewAdmitted(),
				nmcorrelation.NewCohort(),
				transport.NewFan(
					nomagique.NewNumber(nmcorrelation.NewPeerEnergy(), fixed, peerHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewCohortSigned(), fixed, cohortSignedHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewCohortAbsolute(), fixed, cohortAbsHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewPeerCount(), fixed, peerCountHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewEffectivePeers(), fixed, effectiveHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewDispersion(), fixed, dispersionHold, transport.NewDiscard()),
				),
				transport.NewDiscard(),
			))

			register(peerEnergy, nil)
			register(cohortSigned, nil)
			register(cohortAbs, nil)
			register(peerCount, nil)
			register(effective, nil)
			register(dispersion, nil)
		}

		signed := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(
				nmcorrelation.NewTick(),
				nmcorrelation.NewPairs(algo.NewHayashiYoshida(), measured, reference),
				transport.NewFan(pairBranches...),
				nmcorrelation.NewSigned(),
				transport.NewFan(
					nomagique.NewNumber(
						nmcorrelation.NewFisherEstimator(),
						transport.NewFan(
							nomagique.NewNumber(nmcorrelation.NewFisherBaseline(), fixed, baselineHold, transport.NewDiscard()),
							nomagique.NewNumber(nmcorrelation.NewFisherDivergence(), fixed, divergenceHold, transport.NewDiscard()),
							nomagique.NewNumber(nmcorrelation.NewFisherZScore(), fixed, zscoreHold, transport.NewDiscard()),
						),
						transport.NewDiscard(),
					),
					transport.NewIO[any](nil, nil),
				),
				fixed, signedHold,
			),
		)

		register(signed, symbolLastTime)
		register(count, nil)
		register(absolute, nil)
		register(covariance, nil)
		register(refEnergy, nil)
		register(measEnergy, nil)
		register(refRate, nil)
		register(measRate, nil)
		register(density, nil)
		register(measReturns, nil)
		register(refReturns, nil)
		register(overlap, nil)
		register(shared, nil)
		register(pValue, nil)
		register(stdError, nil)
		register(relative, nil)
		register(baseline, nil)
		register(divergence, nil)
		register(zscore, nil)
		register(velocity, nil)
		register(energyBase, nil)
		register(energyDiv, nil)
		register(energyZ, nil)
		register(energyVel, nil)
	}

	ticker := &Ticker{grid: grid}
	ticker.System = runtime.NewSystem(ctx, "correlation:ticker", ticker)
	ticker.Transition(runtime.READY)
	return ticker
}

/*
Next reads retained metric observations from the grid. It does not admit
market data or advance metric state.
*/
func (ticker *Ticker) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in != nil {
			for range in {
			}
		}

		if ticker.Status() != runtime.READY {
			errnie.Warn(ticker.Name() + ": Next called before READY; dropping event")
			return
		}

		address := transport.NewAddress[*geometry.Coordinate]()
		query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			address, core.Read,
		)

		for out := range ticker.grid.Next(query.Next(nil)) {
			if !yield(out) {
				return
			}
		}
	}
}
