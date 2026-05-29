package trader

import (
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"

	. "github.com/smartystreets/goconvey/convey"
)

func perspectiveTestMeasurement(index int) engine.Measurement {
	return engine.Measurement{
		Source:     "hawkes",
		Regime:     "cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.3,
		Last:       float64(100 + index),
	}
}

func TestPerspectivePruneMeasurementsAtLimit(t *testing.T) {
	Convey("Given a perspective at the measurement cap", t, func() {
		limit := config.System.MaxPerspectiveMeasurements
		perspective := NewPerspective(nil)

		for index := 0; index < limit; index++ {
			perspective.AddMeasurement(perspectiveTestMeasurement(index))
		}

		Convey("It should stay capped when more measurements arrive", func() {
			perspective.AddMeasurement(perspectiveTestMeasurement(limit))

			So(perspective.measurementCount(), ShouldEqual, limit)
		})
	})
}

func TestPerspectiveConcurrentAddAndActiveCount(t *testing.T) {
	Convey("Given concurrent adds and active measurement counts", t, func() {
		perspective := NewPerspective(nil)
		stop := make(chan struct{})
		var waitGroup sync.WaitGroup

		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			now := time.Now()

			for {
				select {
				case <-stop:
					return
				default:
					_ = perspective.activeMeasurementCount(now)
				}
			}
		}()

		for index := 0; index < 512; index++ {
			perspective.AddMeasurement(perspectiveTestMeasurement(index))
		}

		close(stop)
		waitGroup.Wait()

		So(perspective.measurementCount(), ShouldBeLessThanOrEqualTo, config.System.MaxPerspectiveMeasurements)
	})
}

/*
TestPerspectivePredictAlwaysReturns proves that any non-empty bucket produces
a prediction. Selectivity for trade entry happens downstream — the spec calls
for predictions on every batch so feedback can flow back even when no trade is
opened.
*/
func TestPerspectivePredictAlwaysReturns(t *testing.T) {
	measurement := engine.Measurement{
		Source:     "hawkes",
		Regime:     "cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.3,
		Last:       100,
	}

	perspective := NewPerspective([]engine.Measurement{measurement})
	prediction, err := perspective.Predict(engine.PerspectiveMicrostructure)
	if err != nil {
		t.Fatalf("non-empty bucket should produce a prediction: %v", err)
	}

	if !perspective.Ready {
		t.Fatal("expected perspective marked ready")
	}

	if prediction.Confidence <= 0 {
		t.Fatalf("expected positive fused confidence, got %v", prediction.Confidence)
	}

	if prediction.Perspective.Type != engine.PerspectiveMicrostructure {
		t.Fatalf("expected perspective type preserved, got %v", prediction.Perspective.Type)
	}
}

/*
TestPerspectivePredictEmptyIsNotReady guards the only case where Predict
declines: an empty bucket has nothing to forecast against.
*/
func TestPerspectivePredictEmptyIsNotReady(t *testing.T) {
	perspective := NewPerspective(nil)
	if _, err := perspective.Predict(engine.PerspectiveFlow); err == nil {
		t.Fatal("expected empty perspective to stay not ready")
	}
}
