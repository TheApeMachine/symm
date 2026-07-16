package logic

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type readyStageGate struct{}

func (readyStageGate) Ready(system.StageType) bool {
	return true
}

type scriptedHawkes struct {
	symbol   string
	outcomes []excitation.Outcome
	index    int
}

func (scripted *scriptedHawkes) Symbols() []string {
	if scripted == nil {
		return nil
	}

	return []string{scripted.symbol}
}

func (scripted *scriptedHawkes) Outcome(symbol string) (excitation.Outcome, bool) {
	if scripted == nil || symbol != scripted.symbol || len(scripted.outcomes) == 0 {
		return excitation.Outcome{}, false
	}

	if scripted.index >= len(scripted.outcomes) {
		return scripted.outcomes[len(scripted.outcomes)-1], true
	}

	outcome := scripted.outcomes[scripted.index]
	scripted.index++

	return outcome, true
}

func hawkesOutcome(at time.Time, buyRate, sellRate float64) excitation.Outcome {
	return excitation.Outcome{
		At:              at,
		Horizon:         time.Second,
		EventCount:      8,
		BuyArrivalRate:  buyRate,
		SellArrivalRate: sellRate,
		Maturity:        0.75,
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
		},
		Fit: hawkes.BivariateFit{
			MuX:            buyRate,
			MuY:            sellRate,
			AlphaXX:        buyRate,
			AlphaYY:        sellRate,
			AlphaXY:        buyRate * 0.1,
			AlphaYX:        sellRate * 0.1,
			Beta:           2,
			IntensityX:     buyRate,
			IntensityY:     sellRate,
			SpectralRadius: 0.35,
		},
	}
}

func TestAnalyzerUpdate(t *testing.T) {
	Convey("Given measurements for two symbols", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements,
			&types.Measurement{
				Stream: types.Hawkes, Metric: types.MetricArrivalRate,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
			},
			&types.Measurement{
				Stream: types.PumpDump, Metric: types.MetricRVOL,
				Symbol: "ETH/USD", At: time.Unix(2, 0),
			},
		)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("It should write one symbol-local graph directly onto the thesis", func() {
			graphCount := 0

			thesis.Graphs.Range(func(_, _ any) bool {
				graphCount++

				return true
			})

			So(graphCount, ShouldEqual, 2)
			graph, ok := thesis.Graphs.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(graph.(*types.Graph).Nodes().Len(), ShouldEqual, 1)
			graph, ok = thesis.Graphs.Load("ETH/USD")
			So(ok, ShouldBeTrue)
			So(graph.(*types.Graph).Nodes().Len(), ShouldEqual, 1)
		})
	})

	Convey("Given repeated physical evidence for one symbol", t, func() {
		tree := dmt.NewTree("")
		analyzer := &Analyzer{
			tree:      tree,
			resonance: map[string]*Resonance{"BTC/USD": {}},
			causal:    map[string]*Causal{"BTC/USD": NewCausal("BTC/USD")},
		}
		state := manifold.State{
			Symbol: "BTC/USD", At: time.Unix(1, 0), Duration: time.Second,
			Epoch: 1, ReferencePrice: 100, InvalidReason: manifold.Valid,
			Spread: 0.01, BuyCapacity: 1000, SellCapacity: 1000,
			BuyIntensity: 2, SellIntensity: 1,
			Reading: pmanifold.Reading{
				PressureGradX: 1, Divergence: -1, CoherenceMag2: 1,
				GuidanceSpeed: 1,
			},
		}

		first := types.NewThesis(nil)
		first.Manifold.Store(state.Symbol, state)
		analyzer.Update(first)
		firstValue, found := first.Cognition.Load(state.Symbol)

		Convey("It should expose the cold DMT reading on the Thesis", func() {
			So(found, ShouldBeTrue)
			So(firstValue.(types.Cognition).Ready, ShouldBeFalse)
		})

		state.At = state.At.Add(time.Second)
		state.Epoch++
		second := types.NewThesis(nil)
		second.Manifold.Store(state.Symbol, state)
		analyzer.Update(second)
		secondValue, found := second.Cognition.Load(state.Symbol)

		Convey("It should classify the repeated sequence from existing memory", func() {
			So(found, ShouldBeTrue)
			reading := secondValue.(types.Cognition)
			So(reading.Ready, ShouldBeTrue)
			So(reading.Winner, ShouldEqual, "buy")
			So(reading.Cohort, ShouldEqual, 1)
		})
	})

	Convey("Given successive tick measurements for one symbol", t, func() {
		analyzer := &Analyzer{}
		positive := 0.5
		first := types.NewThesis(nil)
		first.Measurements = append(first.Measurements, &types.Measurement{
			Source: types.SourceHawkes, Stream: types.Hawkes,
			Metric: types.MetricStrength, Subject: types.SubjectTradeArrivals,
			Side: types.SideBuy, Symbol: "BTC/USD", At: time.Unix(1, 0),
			Unit: types.UnitDimensionless, Normalized: &positive,
			Validity: types.MeasurementValidity{State: types.ValidityValid},
		})
		analyzer.Update(first)
		second := types.NewThesis(nil)
		second.Measurements = append(second.Measurements, &types.Measurement{
			Source: types.SourceHawkes, Stream: types.Hawkes,
			Metric: types.MetricStrength, Subject: types.SubjectTradeArrivals,
			Side: types.SideBuy, Symbol: "BTC/USD", At: time.Unix(2, 0),
			Unit: types.UnitDimensionless, Normalized: &positive,
			Validity: types.MeasurementValidity{State: types.ValidityValid},
		})

		analyzer.Update(second)

		Convey("It should keep only the current tick topology on the Thesis", func() {
			value, found := second.Graphs.Load("BTC/USD")
			So(found, ShouldBeTrue)
			evidenceGraph := value.(*types.Graph)
			So(evidenceGraph.Nodes().Len(), ShouldEqual, 1)
			So(evidenceGraph.Edges().Next(), ShouldBeFalse)
		})
	})

	Convey("Given a changing sequence of next-epoch manifold outcomes", t, func() {
		analyzer := &Analyzer{}
		var forecast *types.Forecasts

		for index := 1; index <= 64 && forecast == nil; index++ {
			price := 100 + float64(index) + math.Sin(float64(index))
			state := causalState(time.Unix(int64(index), 0), price, uint64(index))
			state.Reading.PressureGradX += float64(index) / 100
			state.Reading.Divergence += math.Cos(float64(index)) / 100
			state.BuyIntensity += math.Sin(float64(index)) / 10
			state.SellIntensity -= math.Sin(float64(index)) / 10
			thesis := types.NewThesis(nil)
			thesis.Manifold.Store(state.Symbol, state)
			analyzer.Update(thesis)

			if len(thesis.Forecasts) > 0 {
				forecast = &thesis.Forecasts[0]
			}
		}

		Convey("It should publish a calibrated Resonance and Causal forecast", func() {
			So(forecast, ShouldNotBeNil)
			So(forecast.Source, ShouldEqual, "resonance+causal")
			So(forecast.Target, ShouldEqual, "next_l3_epoch_mid_log_return")
			So(forecast.ModelVersion, ShouldEqual, "resonance_return_head_v1")
			So(forecast.CalibrationSamples, ShouldBeGreaterThan, 0)
			So(forecast.IncrementalMSE, ShouldBeGreaterThanOrEqualTo, 0)
			So(forecast.Uncertainty, ShouldBeGreaterThanOrEqualTo, 0)
			So(forecast.Ready, ShouldBeTrue)
			So(forecast.Calibrated, ShouldBeTrue)
			So(forecast.FrictionReady, ShouldBeFalse)
		})
	})
}

func TestAnalyzerUpdateComposesRelationships(t *testing.T) {
	Convey("Given comparable typed measurements on one thesis", t, func() {
		thesis := types.NewThesis(nil)
		positive := 0.5
		negative := -0.5
		thesis.Measurements = append(thesis.Measurements,
			&types.Measurement{
				Source: types.SourceHawkes, Stream: types.Hawkes,
				Metric: types.MetricStrength, Subject: types.SubjectTradeArrivals,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
				Unit: types.UnitDimensionless, Normalized: &positive,
				Validity: types.MeasurementValidity{State: types.ValidityValid},
			},
			&types.Measurement{
				Source: types.SourcePumpDump, Stream: types.PumpDump,
				Metric: types.MetricStrength, Subject: types.SubjectTradeArrivals,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
				Unit: types.UnitDimensionless, Normalized: &negative,
				Validity: types.MeasurementValidity{State: types.ValidityValid},
			},
		)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("It should retain the contradiction on the symbol graph", func() {
			graph, ok := thesis.Graphs.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			evidenceGraph := graph.(*types.Graph)
			edges := evidenceGraph.Edges()
			So(edges.Next(), ShouldBeTrue)
			edge := edges.Edge()
			lines := evidenceGraph.Lines(edge.From().ID(), edge.To().ID())
			So(lines.Next(), ShouldBeTrue)
			So(lines.Line().(*types.Edge).Type, ShouldEqual, types.Contradicts)
		})
	})
}

func BenchmarkAnalyzerUpdate(b *testing.B) {
	analyzer := &Analyzer{}

	for b.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements, &types.Measurement{
			Stream: types.Hawkes, Metric: types.MetricArrivalRate,
			Symbol: "BTC/USD", At: time.Unix(1, 0),
		})
		analyzer.Update(thesis)
	}
}

func BenchmarkAnalyzerCognize(b *testing.B) {
	analyzer := &Analyzer{tree: dmt.NewTree("")}
	state := manifold.State{
		Symbol: "BTC/USD", At: time.Unix(1, 0), Duration: time.Second,
		Epoch: 1, ReferencePrice: 100, InvalidReason: manifold.Valid,
		Spread: 0.01, BuyCapacity: 1000, SellCapacity: 1000,
		BuyIntensity: 2, SellIntensity: 1,
		Reading: pmanifold.Reading{
			PressureGradX: 1, Divergence: -1, CoherenceMag2: 1,
			GuidanceSpeed: 1,
		},
	}

	for b.Loop() {
		analyzer.cognize(types.NewThesis(nil), state)
	}
}
