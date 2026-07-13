package tests

import (
	"iter"
	"math/rand/v2"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
Engine lazily steps a Kraken wire frame forward over a horizon.
Drift trends price fields, jitter adds bounded noise, volume grows each
step, and interval advances row timestamps.
*/
type Engine struct {
	horizon   int
	drift     float64
	jitter    float64
	volumeAdd float64
	interval  time.Duration
	seed      uint64
}

/*
NewEngine creates a sequencer with a positive horizon.
*/
func NewEngine(horizon int) *Engine {
	if horizon < 1 {
		panic(errnie.Err(errnie.Validation, "tests: engine horizon must be positive", nil))
	}

	return &Engine{horizon: horizon, seed: 1}
}

/*
Drift sets the per-step multiplicative price trend.
*/
func (engine *Engine) Drift(rate float64) *Engine {
	engine.drift = rate

	return engine
}

/*
Jitter sets bounded random noise around the drifted price each step.
*/
func (engine *Engine) Jitter(amplitude float64) *Engine {
	engine.jitter = amplitude

	return engine
}

/*
VolumeAdd adds a fixed amount to volume fields on every step.
*/
func (engine *Engine) VolumeAdd(step float64) *Engine {
	engine.volumeAdd = step

	return engine
}

/*
Interval advances row timestamps on every step.
*/
func (engine *Engine) Interval(step time.Duration) *Engine {
	engine.interval = step

	return engine
}

/*
Seed fixes jitter for repeatable sequences.
*/
func (engine *Engine) Seed(seed uint64) *Engine {
	engine.seed = seed

	return engine
}

/*
Run yields raw Kraken frames lazily from one embedded base payload.
*/
func (engine *Engine) Run(base []byte) iter.Seq[[]byte] {
	template := decodeFrame(base)
	random := rand.New(rand.NewPCG(engine.seed, engine.seed))

	return func(yield func([]byte) bool) {
		for index := range engine.horizon {
			step := cloneFrame(template)
			priceMul := engine.priceMultiplier(index, random)

			engine.apply(step, index, priceMul)

			if !yield(marshalFrame(step)) {
				return
			}
		}
	}
}

/*
Frames yields typed frames from one embedded base payload.
*/
func (engine *Engine) Frames(base []byte) iter.Seq[Frame] {
	return FrameSequence(engine.Run(base))
}

func (engine *Engine) apply(frame map[string]any, index int, priceMul float64) {
	scaleFrameFields(frame, priceMul, 1, engine.volumeAdd, index)

	for _, row := range frameRows(frame) {
		timestamp, ok := row["timestamp"].(string)

		if !ok || timestamp == "" || engine.interval <= 0 {
			continue
		}

		row["timestamp"] = advanceTimestamp(
			timestamp,
			engine.interval*time.Duration(index+1),
		)
	}
}

func (engine *Engine) priceMultiplier(index int, random *rand.Rand) float64 {
	trend := 1 + engine.drift*float64(index+1)

	if engine.jitter <= 0 {
		return trend
	}

	noise := 1 + engine.jitter*(random.Float64()*2-1)

	return trend * noise
}

func decodeFrame(base []byte) map[string]any {
	var frame map[string]any

	if err := sonic.Unmarshal(base, &frame); err != nil {
		panic(errnie.Err(errnie.Validation, "tests: engine decode failed", err))
	}

	return frame
}
