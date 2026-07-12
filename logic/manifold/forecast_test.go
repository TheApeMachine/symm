package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

func TestForecastModelUpdate(t *testing.T) {
	Convey("Given a next-event manifold forecast model", t, func() {
		model, err := NewForecastModel(ForecastConfig{
			InitialVariance:  1,
			ForgettingFactor: 1,
		})
		So(err, ShouldBeNil)

		first, ready, err := model.Update(forecastState(1, 100))
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)

		Convey("It should emit a prior forecast before seeing the next target", func() {
			So(first.Target, ShouldEqual, forecastTarget)
			So(first.SourceEpoch, ShouldEqual, uint64(1))
			So(first.CalibrationSamples, ShouldEqual, uint64(0))
		})

		Convey("When the next L3 epoch reveals the target", func() {
			second, ready, err := model.Update(forecastState(2, 101))

			Convey("Then the prior prediction is scored before model training", func() {
				So(err, ShouldBeNil)
				So(ready, ShouldBeTrue)
				So(second.CalibrationSamples, ShouldEqual, uint64(1))
				So(second.At, ShouldEqual, time.Unix(2, 0))
			})
		})
	})
}

func BenchmarkForecastModelUpdate(b *testing.B) {
	model, err := NewForecastModel(ForecastConfig{
		InitialVariance:  1,
		ForgettingFactor: 1,
	})

	if err != nil {
		b.Fatal(err)
	}

	for index := 1; b.Loop(); index++ {
		_, _, _ = model.Update(forecastState(uint64(index), 100+float64(index%10)/100))
	}
}

func forecastState(epoch uint64, midPrice float64) State {
	return State{
		Source:               "manifold",
		Symbol:               "BTC/USD",
		At:                   time.Unix(int64(epoch), 0),
		Epoch:                epoch,
		Ready:                true,
		BestBid:              midPrice - 0.5,
		BestAsk:              midPrice + 0.5,
		MidPrice:             midPrice,
		VisibleMass:          1,
		ConservationResidual: 0,
		BidTouchDensity:      0.6,
		AskTouchDensity:      0.4,
		StressAnisotropy:     0.1,
		DeltaT:               1,
		Subdivisions:         1,
		PriceScale:           1,
		SizeScale:            1,
		Reading: pmanifold.Reading{
			PressureGradX:    0.1,
			PressureGradNorm: 0.1,
			Divergence:       0.01,
			CoherenceMag2:    0.2,
			GuidanceSpeed:    0.03,
			ViscosityProxy:   0.1,
		},
	}
}
