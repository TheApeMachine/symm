package trader

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/price"
)

/*
Crypto combines measurements into perspectives, records predictions, and enters trades.
*/
type Crypto struct {
	ctx              context.Context
	cancel           context.CancelFunc
	pool             *qpool.Q
	broadcasts       map[string]*qpool.BroadcastGroup
	subscribers      map[string]*qpool.Subscriber
	ui               *qpool.BroadcastGroup
	wallet           *Wallet
	predictions      *price.Prediction
	portfolioRisk    *PortfolioRisk
	kellySizer       *KellySizer
	sourceConfidence map[string]*adaptive.EMA
	restingEntries   map[string]restingEntry
	pulses           int
	seq              int
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *Wallet,
	predictions *price.Prediction,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:              ctx,
		cancel:           cancel,
		pool:             pool,
		broadcasts:       make(map[string]*qpool.BroadcastGroup),
		subscribers:      make(map[string]*qpool.Subscriber),
		wallet:           wallet,
		predictions:      predictions,
		portfolioRisk:    NewPortfolioRisk(),
		kellySizer:       NewKellySizer(engine.DefaultCalibrationParams()),
		sourceConfidence: make(map[string]*adaptive.EMA),
		restingEntries:   make(map[string]restingEntry),
	}

	crypto.subscribers["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond).
		Subscribe("crypto:measurements", 128)

	crypto.subscribers["exits"] = pool.CreateBroadcastGroup("exits", 10*time.Millisecond).
		Subscribe("crypto:exits", 128)

	crypto.subscribers["feedback"] = pool.CreateBroadcastGroup("feedback", 10*time.Millisecond).
		Subscribe("crypto:feedback", 128)

	crypto.subscribers["tick"] = pool.CreateBroadcastGroup("tick", 10*time.Millisecond).
		Subscribe("crypto:tick", 128)

	crypto.broadcasts["confidence"] = pool.CreateBroadcastGroup("confidence", 10*time.Millisecond)
	crypto.broadcasts["wallet"] = pool.CreateBroadcastGroup("wallet", 10*time.Millisecond)
	crypto.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":         ctx,
		"cancel":      cancel,
		"pool":        pool,
		"wallet":      wallet,
		"predictions": predictions,
	})) != nil {
		return nil
	}

	return crypto
}

func (crypto *Crypto) Start() error {
	crypto.sendWallet()
	return nil
}

/*
ResendWallet publishes the current wallet snapshot after the UI hub is listening.
*/
func (crypto *Crypto) ResendWallet() {
	crypto.sendWallet()
}

func (crypto *Crypto) State() engine.State {
	return engine.READY
}

func (crypto *Crypto) Tick() error {
	for {
		if crypto.tryScorePendingMeasurements() {
			continue
		}

		select {
		case <-crypto.ctx.Done():
			crypto.cancel()
			return crypto.ctx.Err()
		case value := <-crypto.subscribers["feedback"].Incoming:
			feedback, ok := value.Value.(engine.PredictionFeedback)

			if !ok {
				errnie.Error(fmt.Errorf("invalid prediction feedback: %v", value.Value))
				break
			}

			crypto.kellySizer.ApplyFeedback(feedback)
		case value := <-crypto.subscribers["measurements"].Incoming:
			if err := crypto.ingestMeasurement(value.Value); err != nil {
				errnie.Error(err)
			}
		case value := <-crypto.subscribers["exits"].Incoming:
			exit, ok := value.Value.(engine.Exit)

			if !ok {
				errnie.Error(fmt.Errorf("invalid exit data: %v", value.Value))
				break
			}

			if err := crypto.handleExit(exit); err != nil {
				errnie.Error(err)
			}
		case value := <-crypto.subscribers["tick"].Incoming:
			row, ok := value.Value.(market.TickerRow)

			if !ok {
				errnie.Error(fmt.Errorf("invalid ticker row: %v", value.Value))
				break
			}

			if err := crypto.observeTicker(row); err != nil {
				errnie.Error(err)
			}
		}
	}
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
