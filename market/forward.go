package market

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
)

const forwardPendingCap = 64

type forwardPending struct {
	symbol      string
	source      logic.SourceType
	anchorPrice float64
	forecastBps float64
	openedAt    time.Time
}

type forwardPendingQueue struct {
	mu    sync.Mutex
	items []forwardPending
}

type forwardCalibrator struct {
	mu         sync.Mutex
	mse        float64
	scale      float64
	bias       float64
	samples    int
	meanReturn float64
	m2Return   float64
	slopeSeen  bool
}

/*
SettleForwardFeedback matures pending labels against mark prices and refreshes
stored measurement calibration.
*/
func (story *Story) SettleForwardFeedback(
	now time.Time,
	mark func(string) (float64, bool),
) {
	if story == nil || mark == nil || now.IsZero() {
		return
	}

	window := story.forwardWindow()
	alpha := story.forwardSlopeAlpha()

	story.forwardPending.Range(func(_, value any) bool {
		queue := value.(*forwardPendingQueue)

		for _, pending := range queue.settle(now, window) {
			markPrice, ok := mark(pending.symbol)

			if !ok || markPrice <= 0 {
				queue.requeue(pending)

				continue
			}

			realizedBps := forwardReturnBps(pending.anchorPrice, markPrice)
			story.calibratorFor(pending.symbol, pending.source).observe(
				pending.forecastBps,
				realizedBps,
				alpha,
			)
		}

		return true
	})
}

/*
FeedbackFor exposes per-source calibration stats for dashboards and tests.
*/
func (story *Story) FeedbackFor(symbol string, source logic.SourceType) *Feedback {
	if story == nil || symbol == "" || source == "" {
		return nil
	}

	raw, ok := story.forwardCal.Load(forwardSourceKey(symbol, source))

	if !ok {
		return nil
	}

	feedback := raw.(*forwardCalibrator).snapshot(symbol)

	if feedback.Samples == 0 {
		return nil
	}

	return feedback
}

func (story *Story) calibratorFor(symbol string, source logic.SourceType) *forwardCalibrator {
	key := forwardSourceKey(symbol, source)
	raw, _ := story.forwardCal.LoadOrStore(key, &forwardCalibrator{scale: 1})

	return raw.(*forwardCalibrator)
}

func (story *Story) forwardWindow() time.Duration {
	window := viper.GetDuration("market.story.measurement_max_age")

	if window <= 0 {
		window = 30 * time.Second
	}

	return window
}

func (story *Story) forwardSlopeAlpha() float64 {
	alpha := viper.GetFloat64("market.story.forward_return_slope_alpha")

	if alpha <= 0 || alpha > 1 {
		alpha = 0.05
	}

	return alpha
}

func forwardSourceKey(symbol string, source logic.SourceType) string {
	return symbol + "\x00" + string(source)
}

func forwardReturnBps(anchorPrice, markPrice float64) float64 {
	if anchorPrice <= 0 || markPrice <= 0 {
		return 0
	}

	return (markPrice - anchorPrice) / anchorPrice * 10000
}

func (queue *forwardPendingQueue) requeue(pending forwardPending) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	queue.items = append(queue.items, pending)

	if len(queue.items) > forwardPendingCap {
		queue.items = queue.items[len(queue.items)-forwardPendingCap:]
	}
}

func (queue *forwardPendingQueue) settle(now time.Time, window time.Duration) []forwardPending {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	if len(queue.items) == 0 {
		return nil
	}

	settled := make([]forwardPending, 0, len(queue.items))
	kept := queue.items[:0]

	for _, pending := range queue.items {
		if now.Sub(pending.openedAt) < window {
			kept = append(kept, pending)

			continue
		}

		settled = append(settled, pending)
	}

	queue.items = kept

	return settled
}
