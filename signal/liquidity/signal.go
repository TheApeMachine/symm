package liquidity

import (
	"context"
	"io"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	marketsection "github.com/theapemachine/symm/market"
	"gonum.org/v1/gonum/stat"
)

/*
Signal identifies opportunities in thin markets by ranking quote volume against peers.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriteCloser
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
}

/*
NewSignal composes the depth pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	})

	if err != nil {
		cancel()

		return nil
	}

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		tree:         tree,
		CrossSection: crossSection,
		algo: nomagique.Number(
			equation.NewDepth(),
			probability.NewClassifier(
				datura.Acquire("liquidity-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"scarcityScore", "medianScore", "depthScore"},
				}),
			),
		),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil || signal.CrossSection == nil {
		return nil
	}

	row, rowErr := marketsection.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(signal.CrossSection.Observe(row)) != nil {
		return nil
	}

	peers := signal.CrossSection.Volumes()

	if len(peers) < 2 {
		return nil
	}

	features := depthFeatureBatch(row.Volume, peers)
	stored := datura.Acquire("liquidity-depth", datura.APPJSON)
	stored.WithPayload(equation.MarshalFeaturesPayload(features))

	if errnie.Error(transport.NewFlipFlop(
		stored, signal.algo,
	)) != nil {
		stored.Release()

		return nil
	}

	confidence := datura.Peek[float64](stored, "output", "confidence")
	uniformConfidence := 1.0 / 3.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		stored.Release()

		return nil
	}

	return stored
}

func depthFeatureBatch(quoteVolume float64, peers []float64) []float64 {
	peerCount := len(peers)
	relativeVolume := 0.0
	baselineReady := 0.0
	sortedPeers := append([]float64(nil), peers...)
	sort.Float64s(sortedPeers)
	median := stat.Quantile(0.5, stat.LinInterp, sortedPeers, nil)

	if median > 0 {
		relativeVolume = quoteVolume / median
		baselineReady = 1
	}

	batch := make([]float64, 0, 2+peerCount+2)
	batch = append(batch, quoteVolume, float64(peerCount))
	batch = append(batch, peers...)
	batch = append(batch, relativeVolume, baselineReady)

	return batch
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
