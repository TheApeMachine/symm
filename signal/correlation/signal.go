package correlation

import (
	"encoding/json"
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
	signalpool "github.com/theapemachine/symm/signal"
)

const (
	// gridBars is how many recent bar returns feed each SimHash hyperplane dot product.
	gridBars = 32
	// hashBits is the fingerprint width: one uint64 signature per coin per batch.
	hashBits = 64
	// correlationBatchInterval is the cross-section window. Trades accumulate per
	// symbol and the herd pass runs once per interval — correlation is a property
	// of the cross-section, not of a single print.
	correlationBatchInterval = 250 * time.Millisecond
	// energyFloor excludes coins whose variance is below this fraction of the slow
	// market-energy baseline — dead-flat and illiquid coins cannot vote as herd.
	energyFloor     = 0.0625
	rawSubscriberID = "signal/correlation:raw"
)

/*
Signal measuring cross-asset "herd behavior" via the dominant eigenmode of the
return field — read cheaply as bit-agreement with the market's majority
fingerprint, so the whole universe is classified in one O(n) pass per tick with
no pairwise correlation matrix.

  - Fingerprint: each coin's recent movement is stamped into one 64-bit
    signature through a fixed set of random hyperplanes (SimHash).
  - Market mode: the bit-by-bit majority vote across all signatures is the
    dominant shared direction — "the market."
  - Correlation: a coin's bit-agreement with that majority (a single popcount)
    is how hard it is herding; anti-agreement is divergence.
  - Energy: each coin's exponentially-weighted return variance, normalised by a
    slow market-energy baseline, separates active regimes from quiet noise.

| Category         | Correlation | Variance | Market "Feel"   |
|:-----------------|:------------|:---------|:----------------|
| Systemic Herd    | High >0.85  | High     | Global Beta     |
| Decoupled Alpha  | Low         | High     | Unique Driver   |
| Stochastic Noise | Low         | Low      | Quiet           |
| Divergent Stress | Negative    | High     | Contrarian Move |
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	symbols       sync.Map
	planes        [hashBits][gridBars]float64
	marketEnergy  *adaptive.EMA
	categories    map[string]perspectives.CategoryType
	activeScratch []live
	floor         *adaptive.SNRField
}

/*
NewSignal wires the measurements broadcast and installs one fixed random
hyperplane set. The seed is fixed so every coin is stamped with the same
projection — otherwise bit agreement would be meaningless.
*/
func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		broadcasts:   make(map[string]*qpool.BroadcastGroup),
		subscribers:  make(map[string]*qpool.Subscriber),
		marketEnergy: adaptive.NewEMA(0),
		categories: map[string]perspectives.CategoryType{
			"divergent_stress": perspectives.CategoryDivergentStress,
			"stochastic_noise": perspectives.CategoryStochasticNoise,
			"decoupled_alpha":  perspectives.CategoryDecoupledAlpha,
			"systemic_herd":    perspectives.CategorySystemicHerd,
		},
		floor: adaptive.NewSNRField(),
	}

	// Fixed seed: one shared projection for the whole universe.
	rng := rand.New(rand.NewSource(1))

	for planeIndex := range signal.planes {
		for barIndex := range signal.planes[planeIndex] {
			signal.planes[planeIndex][barIndex] = float64(rng.Intn(2)*2 - 1)
		}
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	activate.Boot("signal/correlation ready")

	return signal
}

/*
Tick accumulates the latest trade price per symbol and runs one herd pass per
batch tick. Per-trade processing would restamp fingerprints on every print;
correlationBatchInterval batches enough cross-section activity to be stable.
*/
func (signal *Signal) Tick() error {
	batch := time.NewTicker(correlationBatchInterval)
	defer batch.Stop()

	latest := make(map[string]float64)

	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(map[string]any)

			if !ok {
				continue
			}

			channel, _ := envelope["channel"].(string)
			rawData, _ := envelope["data"].(json.RawMessage)
			sm := &public.SocketMessage{Channel: channel, Data: rawData}

			switch channel {
			case public.TradesChannel:
				trades := signalpool.GetTrades(sm)

				for _, trade := range trades {
					if trade.Price > 0 {
						latest[trade.Symbol] = trade.Price
					}
				}
			}
		case <-batch.C:
			if len(latest) == 0 {
				continue
			}

			if err := signal.process(latest); err != nil {
				errnie.Error(err, "correlation: process")
				continue
			}

			clear(latest)
		}
	}
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
