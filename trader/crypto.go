package trader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/price"
)

/*
Crypto combines measurements into perspectives, records predictions, and enters trades.
*/
type Crypto struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	pool             *qpool.Q
	broadcasts       map[string]*qpool.BroadcastGroup
	subscribers      map[string]*qpool.Subscriber
	wallet           *Wallet
	perspectives     []*Perspective
	predictions      []*engine.Prediction
	portfolioRisk    *PortfolioRisk
	kellySizer       *KellySizer
	sourceConfidence map[string]*adaptive.EMA
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
		perspectives:     make([]*Perspective, 0),
		predictions:      make([]*engine.Prediction, 0),
		portfolioRisk:    NewPortfolioRisk(),
		kellySizer:       NewKellySizer(engine.DefaultCalibrationParams()),
		sourceConfidence: make(map[string]*adaptive.EMA),
	}

	for _, channel := range []string{"measurements", "ui"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe(channel, 128)
	}

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

func (crypto *Crypto) State() engine.State {
	return engine.READY
}

func (crypto *Crypto) Tick() error {
	var (
		wg          sync.WaitGroup
		measurement engine.Measurement
		prediction  engine.Prediction
	)

	wg.Go(func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
			case raw, ok := <-crypto.subscribers["measurements"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("crypto measurements channel closed"))
				}

				if measurement, ok = raw.Value.(engine.Measurement); !ok {
					errnie.Error(fmt.Errorf("invalid measurement: %v", raw))
					return
				}

				// Check if ground truth has caught up with any predictions
				if len(crypto.predictions) > 0 {
					due := crypto.predictions[len(crypto.predictions)-1]

					if due.DueAt.Before(time.Now()) {
						lead := leadMeasurement(due.Perspective.Measurements)

						if len(measurement.Pairs) > 0 && len(lead.Pairs) > 0 &&
							measurement.Pairs[0].Wsname == lead.Pairs[0].Wsname {
							anchor := anchorPrice(lead)
							lastPrice := anchorPrice(measurement)

							if anchor > 0 && lastPrice > 0 {
								due.ActualReturn = float64(due.Direction) * (lastPrice - anchor) / anchor
								due.Err = due.ExpectedReturn - due.ActualReturn

								crypto.broadcasts["feedback"].Send(&qpool.QValue[any]{Value: map[string]any{
									"event":       "feedback",
									"perspective": due.Perspective.Measurements[0].Pairs[0].Wsname,
									"ts":          time.Now().UTC().Format(time.RFC3339Nano),
								}})
							}
						}

						crypto.predictions = crypto.predictions[:len(crypto.predictions)-1]
					}
				}

				// Check if the last perspective is ready
				if len(crypto.perspectives) > 0 {
					if !crypto.perspectives[len(
						crypto.perspectives,
					)-1].Ready {
						// Keep adding measurements to the last perspective until it is ready
						crypto.perspectives[len(
							crypto.perspectives,
						)-1].AddMeasurement(measurement)
						continue
					}
				}

				// Create a new perspective and add the measurement to it
				crypto.perspectives = append(
					crypto.perspectives, NewPerspective(
						[]engine.Measurement{measurement},
					),
				)

				// Try to predict the perspective
				if prediction, crypto.err = crypto.perspectives[len(
					crypto.perspectives,
				)-1].Predict(); crypto.err != nil {
					errnie.Error(crypto.err)
					return
				}

				crypto.predictions = append(
					crypto.predictions, &prediction,
				)
			}
		}
	})

	wg.Wait()

	return nil
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}
