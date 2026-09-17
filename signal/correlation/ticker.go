package correlation

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
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

	lastHold := store.NewRetained[float64]()
	lastStages := []core.Primitive{nmcorrelation.NewTick(), nmcorrelation.NewLastPrice(), lastHold}

	if measured != "" {
		lastStages = []core.Primitive{
			nmcorrelation.NewTick(),
			nmcorrelation.NewMatch(measured),
			nmcorrelation.NewLastPrice(),
			lastHold,
		}
	}

	lastPrice := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(lastStages...))
	register(lastPrice, symbolLast)

	if measured != "" && reference != "" {
		countHold := store.NewRetained[float64]()
		signedHold := store.NewRetained[float64]()
		absoluteHold := store.NewRetained[float64]()
		covarianceHold := store.NewRetained[float64]()
		refEnergyHold := store.NewRetained[float64]()
		measEnergyHold := store.NewRetained[float64]()
		refRateHold := store.NewRetained[float64]()
		measRateHold := store.NewRetained[float64]()
		densityHold := store.NewRetained[float64]()
		measReturnsHold := store.NewRetained[float64]()
		refReturnsHold := store.NewRetained[float64]()
		overlapHold := store.NewRetained[float64]()
		sharedHold := store.NewRetained[float64]()
		pValueHold := store.NewRetained[float64]()
		errorHold := store.NewRetained[float64]()
		relativeHold := store.NewRetained[float64]()
		baselineHold := store.NewRetained[float64]()
		divergenceHold := store.NewRetained[float64]()
		zscoreHold := store.NewRetained[float64]()
		velocityHold := store.NewRetained[float64]()
		energyBaseHold := store.NewRetained[float64]()
		energyDivHold := store.NewRetained[float64]()
		energyZHold := store.NewRetained[float64]()
		energyVelHold := store.NewRetained[float64]()

		count := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewObservationCount(measured), countHold),
		)
		absolute := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewAbsoluteCorrelation(), absoluteHold),
		)
		covariance := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewPairCovariance(), covarianceHold),
		)
		refEnergy := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewReferenceEnergy(), refEnergyHold),
		)
		measEnergy := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewMeasuredEnergy(), measEnergyHold),
		)
		refRate := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewReferenceEnergyRate(), refRateHold),
		)
		measRate := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewMeasuredEnergyRate(), measRateHold),
		)
		density := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewOverlapDensity(), densityHold),
		)
		measReturns := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewMeasuredReturns(), measReturnsHold),
		)
		refReturns := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewReferenceReturns(), refReturnsHold),
		)
		overlap := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewOverlapCount(), overlapHold),
		)
		shared := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewSharedTime(), sharedHold),
		)
		pValue := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewPValue(), pValueHold),
		)
		stdError := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewStandardError(), errorHold),
		)
		relative := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewEnergyPair(), arithmetic.NewDivide(), relativeHold),
		)
		baseline := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualBaseline(), calculus.NewTanh(), baselineHold),
		)
		divergence := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualDivergence(), divergenceHold),
		)
		zscore := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualZScore(), zscoreHold),
		)
		velocity := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewFisherPoint(), temporal.NewVelocity(), temporal.NewRate(), velocityHold),
		)
		energyBase := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualBaseline(), energyBaseHold),
		)
		energyDiv := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualDivergence(), energyDivHold),
		)
		energyZ := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(statistic.NewResidualZScore(), energyZHold),
		)
		energyVel := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(nmcorrelation.NewEnergyPoint(), temporal.NewVelocity(), temporal.NewRate(), energyVelHold),
		)

		pairBranches := []core.Primitive{
			transport.NewIO[any](nil, nil),
			nomagique.NewNumber(nmcorrelation.NewObservationCount(measured), countHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewAbsoluteCorrelation(), absoluteHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewPairCovariance(), covarianceHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewReferenceEnergy(), refEnergyHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewMeasuredEnergy(), measEnergyHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewReferenceEnergyRate(), refRateHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewMeasuredEnergyRate(), measRateHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewOverlapDensity(), densityHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewMeasuredReturns(), measReturnsHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewReferenceReturns(), refReturnsHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewOverlapCount(), overlapHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewSharedTime(), sharedHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewPValue(), pValueHold, transport.NewDiscard()),
			nomagique.NewNumber(nmcorrelation.NewStandardError(), errorHold, transport.NewDiscard()),
			nomagique.NewNumber(
				nmcorrelation.NewEnergyPair(),
				arithmetic.NewDivide(),
				relativeHold,
				nomagique.NewNumber(
					equation.NewAdaptiveZScore(),
					transport.NewFan(
						nomagique.NewNumber(statistic.NewResidualBaseline(), energyBaseHold, transport.NewDiscard()),
						nomagique.NewNumber(statistic.NewResidualDivergence(), energyDivHold, transport.NewDiscard()),
						nomagique.NewNumber(statistic.NewResidualZScore(), energyZHold, transport.NewDiscard()),
					),
					transport.NewDiscard(),
				),
				transport.NewDiscard(),
			),
			nomagique.NewNumber(
				nmcorrelation.NewFisherPoint(),
				temporal.NewVelocity(),
				temporal.NewRate(),
				velocityHold,
				transport.NewDiscard(),
			),
			nomagique.NewNumber(
				nmcorrelation.NewEnergyPoint(),
				temporal.NewVelocity(),
				temporal.NewRate(),
				energyVelHold,
				transport.NewDiscard(),
			),
		}

		if len(cohort) > 0 {
			peerHold := store.NewRetained[float64]()
			cohortSignedHold := store.NewRetained[float64]()
			cohortAbsHold := store.NewRetained[float64]()
			peerCountHold := store.NewRetained[float64]()
			effectiveHold := store.NewRetained[float64]()
			dispersionHold := store.NewRetained[float64]()

			peerEnergy := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewPeerEnergy(), peerHold),
			)
			cohortSigned := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewCohortSigned(), cohortSignedHold),
			)
			cohortAbs := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewCohortAbsolute(), cohortAbsHold),
			)
			peerCount := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewPeerCount(), peerCountHold),
			)
			effective := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewEffectivePeers(), effectiveHold),
			)
			dispersion := transport.NewConn[*geometry.Coordinate](
				nomagique.NewNumber(nmcorrelation.NewDispersion(), dispersionHold),
			)

			pairBranches = append(pairBranches, nomagique.NewNumber(
				nmcorrelation.NewAdmitted(),
				nmcorrelation.NewCohort(),
				transport.NewFan(
					nomagique.NewNumber(nmcorrelation.NewPeerEnergy(), peerHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewCohortSigned(), cohortSignedHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewCohortAbsolute(), cohortAbsHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewPeerCount(), peerCountHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewEffectivePeers(), effectiveHold, transport.NewDiscard()),
					nomagique.NewNumber(nmcorrelation.NewDispersion(), dispersionHold, transport.NewDiscard()),
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
						calculus.NewAtanh(),
						statistic.NewEstimator(),
						statistic.NewCausalResidual(),
						transport.NewFan(
							nomagique.NewNumber(
								statistic.NewResidualBaseline(),
								calculus.NewTanh(),
								baselineHold,
								transport.NewDiscard(),
							),
							nomagique.NewNumber(statistic.NewResidualDivergence(), divergenceHold, transport.NewDiscard()),
							nomagique.NewNumber(statistic.NewResidualZScore(), zscoreHold, transport.NewDiscard()),
						),
						transport.NewDiscard(),
					),
					transport.NewIO[any](nil, nil),
				),
				signedHold,
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
