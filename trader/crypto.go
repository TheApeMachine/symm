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
	"github.com/theapemachine/symm/wallet"
)

/*
Crypto combines measurements into perspectives, records predictions, and enters trades.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q
	broadcasts   map[string]*qpool.BroadcastGroup
	subscribers  map[string]*qpool.Subscriber
	wallet       *wallet.Wallet
	perspectives []*Perspective
	predictions  []*engine.Prediction
	confidence   map[string]*adaptive.EMA
	kellySizer   *KellySizer
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *wallet.Wallet,
	predictions *price.Prediction,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		broadcasts:   make(map[string]*qpool.BroadcastGroup),
		subscribers:  make(map[string]*qpool.Subscriber),
		wallet:       wallet,
		perspectives: make([]*Perspective, 0),
		predictions:  make([]*engine.Prediction, 0),
		kellySizer:   NewKellySizer(engine.DefaultCalibrationParams()),
		confidence:   make(map[string]*adaptive.EMA),
	}

	for _, channel := range []string{"measurements", "feedback", "ui"} {
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

func (crypto *Crypto) Start() error {
	return nil
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

				if confidence, ok := crypto.confidence[measurement.Source]; !ok {
					confidence = adaptive.NewEMA(0.1)
					crypto.confidence[measurement.Source] = confidence
				}

				if _, err := crypto.confidence[measurement.Source].Next(
					measurement.Confidence,
				); err != nil {
					errnie.Error(err)
				}

				// Check if ground truth has caught up with any predictions
				if len(crypto.predictions) > 0 {
					now := time.Now()
					remaining := crypto.predictions[:0]

					for _, due := range crypto.predictions {
						if !due.DueAt.Before(now) {
							remaining = append(remaining, due)
							continue
						}

						if _, ok := due.Error(measurement); ok {
							lead, _ := due.LeadMeasurement()

							crypto.broadcasts["feedback"].Send(&qpool.QValue[any]{Value: map[string]any{
								"event":       "feedback",
								"perspective": lead.Pairs[0].Wsname,
								"ts":          now.UTC().Format(time.RFC3339Nano),
							}})
						}
					}

					crypto.predictions = remaining
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
