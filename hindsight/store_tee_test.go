package hindsight

import (
	goruntime "runtime"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
)

func TestStoreTee_Push(t *testing.T) {
	Convey("One publisher and one drain preserve observations across ring wraps", t, func() {
		const capacity, observations = 16, 1024
		tee := NewStoreTee(t.Context(), "store-stream", capacity)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		tee.Transition(runtime.READY)
		finished := make(chan struct{})

		go func() {
			defer close(finished)

			for sequence := 1; sequence <= observations; sequence++ {
				for tee.Pending() == capacity {
					goruntime.Gosched()
				}

				tee.Push(&data.Measurement[float64]{SeqIdx: int64(sequence)})
			}
		}()

		for sequence := 1; sequence <= observations; {
			pointer := tee.Next()

			if pointer == nil {
				goruntime.Gosched()
				continue
			}

			So((*data.Measurement[float64])(pointer).SeqIdx, ShouldEqual, sequence)
			sequence++
		}

		<-finished
		So(tee.Pending(), ShouldEqual, 0)
	})
	Convey("Concurrent consumers retain each accepted publication in producer order", t, func() {
		const producers, observations = 8, 64
		tee := NewStoreTee(t.Context(), "concurrent-store", producers*observations)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		tee.Transition(runtime.READY)
		var workers sync.WaitGroup

		for producer := 0; producer < producers; producer++ {
			workers.Go(func() {
				for sequence := 1; sequence <= observations; sequence++ {
					tee.Push(&data.Measurement[float64]{ID: producer, SeqIdx: int64(sequence)})
				}
			})
		}

		workers.Wait()
		sequences := make([]int64, producers)

		for range producers*observations {
			measurement := (*data.Measurement[float64])(tee.Next())
			So(measurement, ShouldNotBeNil)
			sequences[measurement.ID]++
			So(measurement.SeqIdx, ShouldEqual, sequences[measurement.ID])
		}

		So(tee.Pending(), ShouldEqual, 0)
	})

}

func TestStoreTee_Next(t *testing.T) {
	Convey("A storage tee stays empty until explicitly activated", t, func() {
		tee := NewStoreTee(t.Context(), "store-test", 4)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		So(tee.Status(), ShouldEqual, runtime.INIT)
		So(tee.Next() == nil, ShouldBeTrue)
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 1}
		tee.Push(measurement)
		tee.Transition(runtime.READY)
		So(tee.Next() == nil, ShouldBeTrue)

		Convey("Reusing a producer preserves each queued observation", func() {
			measurement.SeqIdx = 1
			tee.Push(measurement)
			measurement.SeqIdx = 2
			tee.Push(measurement)
			So((*data.Measurement[float64])(tee.Next()).SeqIdx, ShouldEqual, 1)
			So((*data.Measurement[float64])(tee.Next()).SeqIdx, ShouldEqual, 2)
		})

		Convey("Successive batches preserve owned observations", func() {
			for batch := 0; batch < 3; batch++ {
				first := data.NewMeasurement[float64]("feed", nil)
				first.Label, first.SeqIdx = "BTC/USD", int64(batch*2+1)
				second := data.NewMeasurement[float64]("feed", nil)
				second.Label, second.SeqIdx = "ETH/USD", first.SeqIdx+1
				tee.Push(first)
				tee.Push(second)
				So((*data.Measurement[float64])(tee.Next()), ShouldResemble, first)
				So((*data.Measurement[float64])(tee.Next()), ShouldResemble, second)
				So(tee.Next() == nil, ShouldBeTrue)
				So(tee.Error(), ShouldBeNil)
			}
		})

		Convey("Observations are tagged in-band with CUSUM excursion departures", func() {
			tee := NewStoreTee(t.Context(), "cusum-tag-test", 8)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			// Step 1: baseline price
			m1 := &data.Measurement[float64]{
				Label:  "BTC/USD",
				SeqIdx: 1,
				Metrics: map[string]data.Metric[float64]{
					"price":  {Raw: 50000.0},
					"spread": {Raw: 2.0},
				},
			}
			tee.Push(m1)
			out1 := (*data.Measurement[float64])(tee.Next())
			So(out1, ShouldNotBeNil)
			So(out1.Metadata["cusum_upper"], ShouldNotBeBlank)

			// Step 2: large upward price excursion exceeding hurdle and threshold
			m2 := &data.Measurement[float64]{
				Label:  "BTC/USD",
				SeqIdx: 2,
				Metrics: map[string]data.Metric[float64]{
					"price":  {Raw: 50100.0},
					"spread": {Raw: 2.0},
				},
			}
			tee.Push(m2)
			out2 := (*data.Measurement[float64])(tee.Next())
			So(out2, ShouldNotBeNil)
			So(out2.Metadata["excursion"], ShouldEqual, "upper")
			So(out2.Metadata["excursion_start"], ShouldEqual, "1")

			// Step 3: peer/logic measurement without price metric at same sequence inherits excursion tag
			m3 := &data.Measurement[float64]{
				Label:  "BTC/USD",
				Source: "cvd",
				SeqIdx: 2,
			}
			tee.Push(m3)
			out3 := (*data.Measurement[float64])(tee.Next())
			So(out3, ShouldNotBeNil)
			So(out3.Metadata["excursion"], ShouldEqual, "upper")
			So(out3.Metadata["excursion_start"], ShouldEqual, "1")
		})

		Convey("Buffered pre-run lead tape and multi-stage observations are tagged when Point B ignites", func() {
			tee := NewStoreTee(t.Context(), "tape-fragment-test")
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			// Sequence 1: Baseline / Lead Tape (both market trade and cvd signal)
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", Source: "public", SeqIdx: 1,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 50000.0}, "spread": {Raw: 1.0}},
			})
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", Source: "cvd", SeqIdx: 1,
			})
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", Source: "resonance", SeqIdx: 1,
			})

			// Sequence 2: Point A Onset (delta +2.0 exceeds hurdle 0.5)
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", Source: "public", SeqIdx: 2,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 50002.0}, "spread": {Raw: 1.0}},
			})
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", Source: "cvd", SeqIdx: 2,
			})

			// Sequence 3: Point B Ignition (delta +3.0 exceeds threshold 2.0)
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", Source: "public", SeqIdx: 3,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 50005.0}, "spread": {Raw: 1.0}},
			})
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", Source: "resonance", SeqIdx: 3,
			})

			// Read all out and verify every measurement across all stages has been tagged
			for range 7 {
				out := (*data.Measurement[float64])(tee.Next())
				So(out, ShouldNotBeNil)
				So(out.Metadata["excursion"], ShouldEqual, "upper")
				So(out.Metadata["excursion_start"], ShouldEqual, "1")
			}
		})

		Convey("Single-tick stop-loss hunter wicks do not terminate an active excursion", func() {
			tee := NewStoreTee(t.Context(), "stoploss-hunter-test", 64)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			// Step 1: Baseline
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 1,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 100.0}, "spread": {Raw: 1.0}},
			})
			// Step 2: Ignition (Point B)
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 2,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 105.0}, "spread": {Raw: 1.0}},
			})
			// Step 3: Run continues to 110
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 3,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 110.0}, "spread": {Raw: 1.0}},
			})
			// Step 4: Single-tick punch down (stop-loss hunter / wick)
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 4,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 101.0}, "spread": {Raw: 1.0}},
			})
			// Step 5: Immediate recovery to higher peak 115!
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 5,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 115.0}, "spread": {Raw: 1.0}},
			})
			// Step 6 & 7: Sustained exhaustion/reversal
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 6,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 106.0}, "spread": {Raw: 1.0}},
			})
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 7,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 105.0}, "spread": {Raw: 1.0}},
			})
			// Tail margin ticks
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 8,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 104.0}, "spread": {Raw: 1.0}},
			})
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 9,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 104.0}, "spread": {Raw: 1.0}},
			})

			var completed *data.Measurement[float64]
			for range 9 {
				out := (*data.Measurement[float64])(tee.Next())
				if out != nil && out.Metadata["excursion_event"] == "completed" {
					completed = out
				}
			}

			So(completed, ShouldNotBeNil)
			// True peak 115 was preserved despite the wick at 101
			So(completed.Metadata["excursion_extremum_price"], ShouldEqual, "115")
			So(completed.Metadata["excursion_category"], ShouldEqual, "upper_profitable")
			So(completed.Metadata["excursion_clears_friction"], ShouldEqual, "true")
			So(completed.Metadata["excursion_position_size"], ShouldEqual, "40")
		})

		Convey("A small upward movement that fails to cover $40 friction is marked upper_unprofitable", func() {
			tee := NewStoreTee(t.Context(), "unprofitable-test", 64)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			// Baseline
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 1,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 1000.0}, "spread": {Raw: 1.0}},
			})
			// Ignition: +3 above hurdle
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 2,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 1003.0}, "spread": {Raw: 1.0}},
			})
			// Reaches 1004 (only +0.1% gain, cannot cover ~0.52% round trip fees)
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 3,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 1004.0}, "spread": {Raw: 1.0}},
			})
			// Sustained reversal back to 1001
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 4,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 1001.0}, "spread": {Raw: 1.0}},
			})
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 5,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 1000.5}, "spread": {Raw: 1.0}},
			})
			tee.Push(&data.Measurement[float64]{
				Label: "BTC/USD", SeqIdx: 6,
				Metrics: map[string]data.Metric[float64]{"price": {Raw: 1000.0}, "spread": {Raw: 1.0}},
			})

			var completed *data.Measurement[float64]
			for i := 0; i < 6; i++ {
				out := (*data.Measurement[float64])(tee.Next())
				if out != nil && out.Metadata["excursion_event"] == "completed" {
					completed = out
				}
			}

			So(completed, ShouldNotBeNil)
			So(completed.Metadata["excursion_category"], ShouldEqual, "upper_unprofitable")
			So(completed.Metadata["excursion_clears_friction"], ShouldEqual, "false")
		})

		Convey("A flatline tape emits a flat fragment with clears_friction=false", func() {
			tee := NewStoreTee(t.Context(), "flat-test", 64)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			// 35 ticks of motionless price
			for seq := 1; seq <= 35; seq++ {
				tee.Push(&data.Measurement[float64]{
					Label: "BTC/USD", SeqIdx: int64(seq),
					Metrics: map[string]data.Metric[float64]{"price": {Raw: 50000.0}, "spread": {Raw: 2.0}},
				})
			}

			var completed *data.Measurement[float64]
			for range 35 {
				out := (*data.Measurement[float64])(tee.Next())
				if out != nil && out.Metadata["excursion_event"] == "completed" {
					completed = out
				}
			}

			So(completed, ShouldNotBeNil)
			So(completed.Metadata["excursion_category"], ShouldEqual, "flat")
			So(completed.Metadata["excursion_clears_friction"], ShouldEqual, "false")
		})

		Convey("Adversarial multi-wick tape with multiple stop-loss sweeps preserves full excursion to true peak", func() {
			tee := NewStoreTee(t.Context(), "multi-wick-test", 128)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			tape := market.NewAdversarialMultiWickTape("BTC/USD", 60000.0, 5.0)
			for _, m := range tape {
				tee.Push(m)
			}

			var completed *data.Measurement[float64]
			for range len(tape) {
				out := (*data.Measurement[float64])(tee.Next())
				if out != nil && out.Metadata["excursion_event"] == "completed" {
					completed = out
				}
			}

			So(completed, ShouldNotBeNil)
			// True peak exceeded $62,500 (+4.1%+) despite 3 separate wicks down
			extremumStr := completed.Metadata["excursion_extremum_price"]
			So(extremumStr, ShouldNotBeBlank)
			So(completed.Metadata["excursion_category"], ShouldEqual, "upper_profitable")
			So(completed.Metadata["excursion_clears_friction"], ShouldEqual, "true")
		})

		Convey("Interleaved multi-asset market tape isolates concurrent symbol state machines without cross-talk", func() {
			tee := NewStoreTee(t.Context(), "interleaved-test", 256)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			tape := market.NewInterleavedMultiAssetTape()
			for _, m := range tape {
				tee.Push(m)
			}

			completedBySymbol := make(map[string]*data.Measurement[float64])
			for range len(tape) {
				out := (*data.Measurement[float64])(tee.Next())
				if out != nil && out.Metadata["excursion_event"] == "completed" {
					completedBySymbol[out.Label] = out
				}
			}

			// BTC completed a profitable upper excursion
			So(completedBySymbol["BTC/USD"], ShouldNotBeNil)
			So(completedBySymbol["BTC/USD"].Metadata["excursion_category"], ShouldEqual, "upper_profitable")
			So(completedBySymbol["BTC/USD"].Metadata["excursion_clears_friction"], ShouldEqual, "true")

			// ETH completed a flatline excursion
			So(completedBySymbol["ETH/USD"], ShouldNotBeNil)
			So(completedBySymbol["ETH/USD"].Metadata["excursion_category"], ShouldEqual, "flat")
			So(completedBySymbol["ETH/USD"].Metadata["excursion_clears_friction"], ShouldEqual, "false")

			// SOL completed a downward excursion
			So(completedBySymbol["SOL/USD"], ShouldNotBeNil)
			So(completedBySymbol["SOL/USD"].Metadata["excursion_category"], ShouldEqual, "downward")
			So(completedBySymbol["SOL/USD"].Metadata["excursion_clears_friction"], ShouldEqual, "false")
		})

		Convey("Chop tape with high-frequency oscillation within noise band is identified as chop", func() {
			tee := NewStoreTee(t.Context(), "chop-test", 64)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			tape := market.NewChopWhipsawTape("DOGE/USD", 0.15, 0.001, 35)
			for _, m := range tape {
				tee.Push(m)
			}

			var completed *data.Measurement[float64]
			for range len(tape) {
				out := (*data.Measurement[float64])(tee.Next())
				if out != nil && out.Metadata["excursion_event"] == "completed" {
					completed = out
				}
			}

			So(completed, ShouldNotBeNil)
			So(completed.Metadata["excursion_category"], ShouldEqual, "chop")
			So(completed.Metadata["excursion_clears_friction"], ShouldEqual, "false")
		})

		Convey("Fast pump captures extensive precursor lead tape in buffer and finishes as upper_profitable", func() {
			tee := NewStoreTee(t.Context(), "fast-pump-test", 128)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			tape := market.NewFastPumpTape("PEPE/USD", 10.0, 0.05, 0.15)
			for _, m := range tape {
				tee.Push(m)
			}

			var completed *data.Measurement[float64]
			leadTaggedCount := 0
			for range len(tape) {
				out := (*data.Measurement[float64])(tee.Next())
				if out != nil {
					if out.Metadata["excursion"] == "upper" && out.SeqIdx <= 16 {
						leadTaggedCount++
					}
					if out.Metadata["excursion_event"] == "completed" {
						completed = out
					}
				}
			}

			So(completed, ShouldNotBeNil)
			So(completed.Metadata["excursion_category"], ShouldEqual, "upper_profitable")
			So(completed.Metadata["excursion_clears_friction"], ShouldEqual, "true")
			// Crucial: The precursor lead tape before ignition was tagged so the system captures the lead-up
			So(leadTaggedCount, ShouldBeGreaterThan, 10)
		})

		Convey("Flash spike and instant dump collapses before exit and is categorized as upper_unprofitable", func() {
			tee := NewStoreTee(t.Context(), "flash-spike-test", 128)
			defer func() { So(tee.Close(), ShouldBeNil) }()
			tee.Transition(runtime.READY)

			tape := market.NewFlashSpikeDumpTape("SHIB/USD", 100.0, 0.5, 0.20)
			for _, m := range tape {
				tee.Push(m)
			}

			var completed *data.Measurement[float64]
			for range len(tape) {
				out := (*data.Measurement[float64])(tee.Next())
				if out != nil && out.Metadata["excursion_event"] == "completed" {
					completed = out
				}
			}

			So(completed, ShouldNotBeNil)
			// Because price immediately dumped back to baseline, the position failed to clear friction
			So(completed.Metadata["excursion_category"], ShouldEqual, "upper_unprofitable")
			So(completed.Metadata["excursion_clears_friction"], ShouldEqual, "false")
		})
	})
}

func BenchmarkStoreTee_Next(b *testing.B) {
	tee := NewStoreTee(b.Context(), "store-benchmark", 4)
	tee.Transition(runtime.READY)
	measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 1}
	

	for b.Loop() {
		tee.Push(measurement)

		if (*data.Measurement[float64])(tee.Next()).SeqIdx != measurement.SeqIdx {
			b.Fatal("observation changed")
		}
	}

	b.StopTimer()

	if err := tee.Close(); err != nil {
		b.Fatal(err)
	}
}
