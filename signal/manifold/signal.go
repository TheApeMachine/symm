package manifold

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
)

/*
Signal ...

X-Axis (Micro): Price vs. Local Depth.
This is the 1D Limit Order Book. It measures local resistance (viscosity) and pressure.

Y-Axis (Meso): Spatial Fragmentation (Derivatives).
Crypto is not a single pipe; it is a network of connected pools.
The Y-axis represents the same asset across different instruments (Spot, Perpetuals, Futures).

Z-Axis (Macro): Cross-Asset Correlation (The Universe).
The Z-axis is ordered by market capitalization and historical beta (e.g., BTC → ETH → SOL → Mid-Caps → Memes).

The Periodic Domain (Torus) = Capital Rotation In crypto, capital is largely a closed-loop system in the short-to-medium term.
Fiat enters the ecosystem, pumps Bitcoin, rotates into Large Caps, diffuses into High-Beta Small Caps, and eventually cycles back into Bitcoin or stablecoins.
Modeling the market as a Torus mathematically enforces this conservation of capital.
The fluid leaving the high-cap boundary wraps around and enters the low-cap boundary.
Pressure Gradients (∇p) = Arbitrage & Basis Along the Y-Axis (Cross-Venue): A pressure gradient is an Arbitrage Spread.
If price spikes on Binance Perp (high pressure) but Kraken Spot lags (low pressure), the fluid solver naturally forces capital to advect across the Y-axis to equalize the pressure.
Along the Z-Axis (Cross-Asset): A pressure gradient is a Beta Dislocation.

If BTC moves 5% and ETH has not moved, a pressure differential forms. Capital flows down the Z-axis to restore the historical correlation matrix.

Particle-In-Cell (PIC) = Whale Tracking vs. Retail Flow The Grid (Eulerian): Represents the continuous, background retail flow, passive market makers, and resting liquidity.
The Particles (Lagrangian): Represent discrete, massive block orders or tracked institutional wallets (e.g., tracking a 10,000 BTC transfer on-chain).
When a "massive particle" enters the grid, your particle_interactions and scatter_sorted kernels distribute its momentum into the surrounding fluid, creating a shockwave.
This perfectly models how a massive market order depletes the order book and causes the surrounding market makers (the grid fluid) to pull their quotes (advection).
The Gross-Pitaevskii Coherence Layer = Systemic Herding Phase Oscillators (e iθ): Individual algorithmic actors or trader cohorts.
Coherence Field (Ψ): The degree to which these actors are synchronized.
When the market is in a "Stochastic Noise" regime, the phases are random, ∣Ψ∣ 2≈0, and the market behaves like a standard, highly viscous gas (mean-reverting).
However, during a breakout or a liquidation cascade, the actors synchronize.
Their phases align.
The non-linear self-interaction term in the GPE solver (g∣Ψ∣ 2) takes over.
The fluid undergoes a phase transition into a superfluid. Viscosity drops to zero.
The guidance equation: v=mℏ∣Ψ∣2+ϵIm(Ψ∗∇Ψ) becomes the literal "trend-following" velocity.
Capital tunnels through liquidity walls with zero resistance because the entire market is pushing in the exact same direction simultaneously.
*/
type Signal struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	tree             *dmt.Tree
	field            *Field
	lastHydrateStamp int64
	featureCache     featureCacheEntry
}

type featureCacheEntry struct {
	scope      string
	eventStamp int64
	pressure   float64
	coherence  float64
	guidance   float64
	viscosity  float64
	ok         bool
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	field := errnie.Does(func() (*Field, error) {
		return NewField()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: failed to create field",
			err,
		))
	}).Value()

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		field:  field,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"book", "trade", "ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil {
			return
		}

		if signal.field == nil {
			return
		}

		channel := datura.Peek[string](datapoint, "channel")

		switch channel {
		case "book":
			signal.observeBookArtifact(datapoint)
			return
		case "trade":
			signal.observeTradeArtifact(datapoint)
			return
		case "ticker":
			signal.observeTickerArtifact(datapoint)
		case "":
			// ponytail: measurement tick without ingest payload
		default:
			return
		}

		if channel == "" {
			scope := datura.Peek[string](datapoint, "data", 0, "symbol")

			if scope == "" {
				scope, _ = datapoint.Scope()
			}

			if scope == "" {
				return
			}

			measurement := signal.measureScope(scope, datapoint)

			if measurement != nil {
				yield(measurement)
			}

			return
		}

		for rowIndex := 0; ; rowIndex++ {
			scope := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

			if scope == "" {
				return
			}

			measurement := signal.measureScope(scope, datapoint)

			if measurement == nil {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) measureScope(scope string, datapoint *datura.Artifact) *datura.Artifact {
	pressure, coherence, guidance, viscosity, ok := signal.resolveFeatures(
		scope,
		time.Unix(0, datapoint.Timestamp()),
	)

	if !ok {
		return nil
	}

	shares := signal.classify(pressure, coherence, guidance, viscosity)

	measurement := datura.Acquire("manifold", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(scope)
	errnie.Error(measurement.SetOrigin(string(logic.SourceManifold)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("pressureGradNorm", pressure)
	measurement.MergeOutput("coherenceMag2", coherence)
	measurement.MergeOutput("guidanceSpeed", guidance)
	measurement.MergeOutput("viscosityProxy", viscosity)
	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	measurement.Merge("classifier.category", manifoldClassifierIndex(shares))
	measurement.Merge("classifier.confidence", confidence)
	measurement.Merge("scope", scope)

	return measurement
}

func (signal *Signal) classify(
	pressure, coherence, guidance, viscosity float64,
) []dist.Share {
	herd := coherence * guidance
	shock := pressure
	drift := guidance / (1 + viscosity)
	noise := viscosity * (1 - coherence)

	return []dist.Share{
		{Key: "herdScore", Category: logic.CategorySystemicHerd, Mass: herd},
		{Key: "shockScore", Category: logic.CategoryLiquidityShock, Mass: shock},
		{Key: "driftScore", Category: logic.CategorySynchronizedDrift, Mass: drift},
		{Key: "noiseScore", Category: logic.CategoryStochasticNoise, Mass: noise},
	}
}

func manifoldClassifierIndex(shares []dist.Share) int {
	order := []logic.CategoryType{
		logic.CategorySystemicHerd,
		logic.CategoryLiquidityShock,
		logic.CategorySynchronizedDrift,
		logic.CategoryStochasticNoise,
	}

	bestIndex := 0
	bestMass := shares[0].Mass

	for index := range shares {
		if shares[index].Mass > bestMass {
			bestMass = shares[index].Mass
			bestIndex = index
		}
	}

	for index, category := range order {
		if shares[bestIndex].Category == category {
			return index + 1
		}
	}

	return 0
}

func (signal *Signal) FieldSnapshot(eventAt time.Time) (map[string]any, error) {
	if signal == nil || signal.field == nil {
		return nil, nil
	}

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	if _, integrateErr := signal.field.integrate(eventAt); integrateErr != nil {
		return nil, integrateErr
	}

	if !signal.field.hasPublishableSnapshot() {
		return nil, nil
	}

	payload, snapshotErr := signal.field.snapshotPayload(eventAt)

	return payload, snapshotErr
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	if signal.field != nil {
		signal.field.closeSolver()
	}

	return err
}
