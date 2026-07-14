package types

import (
	"encoding/json"
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
thesisRecordFixture builds a complete lifecycle record with real Gonum topology
and separate runtime-only state for codec verification and benchmarks.
*/
type thesisRecordFixture struct {
	at time.Time
}

/*
TestThesisMarshalBinaryConcurrentRecordTrade verifies that the durable snapshot
cannot race broker observations arriving on Kraken's reader goroutines.
*/
func TestThesisMarshalBinaryConcurrentRecordTrade(t *testing.T) {
	Convey("Given an active Thesis receiving broker observations", t, func() {
		thesis := NewThesis(nil)
		thesis.TradeJournal = make([]TradeObservation, 0, 8192)
		start := make(chan struct{})
		errs := make(chan error, 1)
		var wait sync.WaitGroup
		wait.Add(2)

		go func() {
			defer wait.Done()
			<-start

			for range 4096 {
				thesis.RecordTrade(TradeObservation{
					Kind: "position_snapshot", Symbol: "BTC/USD",
				})
				runtime.Gosched()
			}
		}()

		go func() {
			defer wait.Done()
			<-start

			for range 512 {
				if _, err := thesis.MarshalBinary(); err != nil {
					errs <- err

					return
				}

				runtime.Gosched()
			}
		}()

		close(start)
		wait.Wait()
		close(errs)

		for err := range errs {
			So(err, ShouldBeNil)
		}
	})
}

/*
thesis creates one populated Thesis whose graph size can represent either the
focused codec behavior or a realistic repeated serialization workload.
*/
func (fixture thesisRecordFixture) thesis(testingTB testing.TB, nodeCount int) *Thesis {
	testingTB.Helper()
	thesis := NewThesis(make(chan []byte, 1))
	thesis.Tick = 47
	thesis.CrossSection.Metrics = []SymbolMetric{
		{
			Symbol: "BTC/USD", At: fixture.at, Volume: 12,
			QuoteNotional: 1200, ExecutableDepth: 500, LatestChange: 0.03,
		},
		{
			Symbol: "ETH/USD", At: fixture.at, Volume: 20,
			QuoteNotional: 800, ExecutableDepth: 320, LatestChange: -0.01,
		},
	}
	thesis.CrossSection.index = map[string]int{"BTC/USD": 0, "ETH/USD": 1}
	evidenceGraph := NewGraph("BTC/USD")
	var previous *Measurement

	for index := range nodeCount {
		measurement := &Measurement{
			Source: SourceType("fixture"), Stream: Hawkes,
			Metric: MetricArrivalRate, Subject: SubjectTradeArrivals,
			Side: SideBuy, Symbol: "BTC/USD",
			At:  fixture.at.Add(time.Duration(index) * time.Second),
			Raw: float64(index+1) / float64(nodeCount),
		}

		if err := evidenceGraph.AddNode(measurement); err != nil {
			testingTB.Fatal(err)
		}

		if previous != nil && !evidenceGraph.Relate(
			MeasurementKey(previous), MeasurementKey(measurement), Supports,
			measurement.At, previous.At,
		) {
			testingTB.Fatal("failed to relate fixture measurements")
		}

		thesis.Measurements = append(thesis.Measurements, measurement)
		previous = measurement
	}

	thesis.Graphs["BTC/USD"] = evidenceGraph
	thesis.Forecasts = []Forecasts{
		{
			Source: "manifold", Symbol: "BTC/USD", At: fixture.at,
			SourceEpoch: 10, HorizonEvents: 2, ExpiresEpoch: 12,
			Target: "next_return", ModelVersion: "model-a", Ready: true,
			Calibrated: true, FrictionReady: true, CalibrationSamples: 64,
			ExpectedReturn: 0.04, ReferencePrice: 100,
			BuyCapacity: 2, SellCapacity: 2, ExpectedFees: 0.001,
			ExpectedSpread: 0.002, ExpectedImpact: 0.001,
			ExpectedAdverseSelection: 0.001, Confidence: 0.8,
		},
		{
			Source: "manifold", Symbol: "BTC/USD", At: fixture.at.Add(time.Second),
			SourceEpoch: 11, HorizonEvents: 2, ExpiresEpoch: 13,
			Target: "next_return", ModelVersion: "model-a", Ready: true,
			Calibrated: true, FrictionReady: true, CalibrationSamples: 65,
			ExpectedReturn: 0.02, ReferencePrice: 103,
			BuyCapacity: 2, SellCapacity: 2, ExpectedFees: 0.001,
			ExpectedSpread: 0.002, ExpectedImpact: 0.001,
			ExpectedAdverseSelection: 0.001, Confidence: 0.7,
		},
	}
	thesis.Decisions = []Decision{
		{
			Action: "enter", Symbol: "BTC/USD", At: fixture.at,
			Utility: 0.035, Alternatives: map[string]float64{"hold": 0, "enter": 0.035},
			ProposedNotional: 50, ProposedQuantity: 0.5, ReferencePrice: 100,
			ValidThroughEpoch: 12, ForecastSource: "manifold",
			ForecastModel: "model-a", ForecastEpoch: 10,
		},
		{
			Action: "exit", Symbol: "BTC/USD", At: fixture.at.Add(2 * time.Second),
			Utility: 0.01, Alternatives: map[string]float64{"hold": -0.01, "exit": 0.01},
			ProposedQuantity: 0.5, ReferencePrice: 103,
			ValidThroughEpoch: 14, ForecastSource: "manifold",
			ForecastModel: "model-a", ForecastEpoch: 12,
		},
	}
	thesis.TradeJournal = []TradeObservation{
		{
			Kind: "execution", Action: "enter", Symbol: "BTC/USD", Side: "buy",
			Status: "filled", OrderID: "order-1", ExecutionID: "fill-1",
			Quantity: "0.5", Price: "100", Cost: "50", Fee: "0.05", At: fixture.at,
		},
		{
			Kind: "execution", Action: "exit", Symbol: "BTC/USD", Side: "sell",
			Status: "filled", OrderID: "order-2", ExecutionID: "fill-2",
			Quantity: "0.5", Price: "103", Cost: "51.5", Fee: "0.05",
			PnL: "1.4", ReturnPct: 0.028, At: fixture.at.Add(2 * time.Second),
		},
	}
	thesis.Lifecycle = map[string]string{"BTC/USD": LifecycleClosed}
	thesis.Findings = []Finding{
		{
			Symbol: "BTC/USD", Component: "forecast", Condition: "edge held",
			Evidence: []string{"fill-1", "fill-2"}, EstimatedEffect: 0.028,
			Uncertainty: 0.01, RequiredValidation: "replay",
		},
	}
	thesis.Hypotheses = []Hypothesis{
		{
			Source: SourceType("pearl"), Symbol: "BTC/USD", At: fixture.at,
			Samples: 64, Ready: true, Claim: "buy pressure precedes lift",
			Treatment: "pressure", Controls: []string{"spread"}, Outcome: "return",
			Association: 0.6, Intervention: 0.5, DoExpectation: 0.4,
			Uplift: 0.1, Counterfactual: 0.3, Confidence: 0.8, Strength: 0.7,
		},
	}
	thesis.Categories = []Category{
		{
			Symbol: "BTC/USD", Type: ForecastEdge, Confidence: 0.8,
			Surprisal: 0.2, Strength: 0.7, Maturity: 0.9,
			Supporting: []string{"fill-1"}, Opposing: []string{"spread"},
			Missing: []string{"longer replay"},
		},
	}
	thesis.Signals.Store("runtime", "excluded")
	thesis.Manifold = []any{"field-state"}
	thesis.Resonance = []any{"persistent-agreement"}
	thesis.Causal = []any{"causal-state"}

	return thesis
}

/*
TestThesisMarshalBinary verifies stable schema bytes and the deliberate
exclusion of connections that belong only to a running process.
*/
func TestThesisMarshalBinary(t *testing.T) {
	Convey("Given a complete Thesis lifecycle case record", t, func() {
		fixture := thesisRecordFixture{at: time.Unix(100, 20).UTC()}
		thesis := fixture.thesis(t, 3)

		Convey("When the same state is marshaled repeatedly", func() {
			first, firstErr := thesis.MarshalBinary()
			second, secondErr := thesis.MarshalBinary()
			topLevel := make(map[string]json.RawMessage)
			decodeErr := json.Unmarshal(first, &topLevel)
			_, hasSignals := topLevel["signals"]
			_, hasUIHub := topLevel["uiHub"]
			_, hasManifold := topLevel["manifold"]
			_, hasResonance := topLevel["resonance"]
			_, hasCausal := topLevel["causal"]

			Convey("Then the versioned JSON is deterministic and runtime state is absent", func() {
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(decodeErr, ShouldBeNil)
				So(json.Valid(first), ShouldBeTrue)
				So(first, ShouldResemble, second)
				So(string(first), ShouldContainSubstring, `"schemaVersion":1`)
				So(hasSignals, ShouldBeFalse)
				So(hasUIHub, ShouldBeFalse)
				So(hasManifold, ShouldBeTrue)
				So(hasResonance, ShouldBeTrue)
				So(hasCausal, ShouldBeTrue)
			})
		})
	})
}

/*
TestRestoreThesis verifies complete domain recovery, derived structure
reconstruction, runtime reset, and rejection of corrupt durable topology.
*/
func TestRestoreThesis(t *testing.T) {
	Convey("Given a serialized Thesis lifecycle case record", t, func() {
		fixture := thesisRecordFixture{at: time.Unix(100, 20).UTC()}
		original := fixture.thesis(t, 3)
		payload, err := original.MarshalBinary()
		So(err, ShouldBeNil)

		Convey("When it is restored with a new runtime connection", func() {
			uiHub := make(chan []byte, 1)
			restored, restoreErr := RestoreThesis(payload, uiHub)
			So(restoreErr, ShouldBeNil)

			Convey("Then every durable field and Gonum relationship is recovered", func() {
				remarshaled, marshalErr := restored.MarshalBinary()
				So(marshalErr, ShouldBeNil)
				So(remarshaled, ShouldResemble, payload)
				So(restored.Tick, ShouldEqual, original.Tick)
				So(restored.CrossSection.Metrics, ShouldResemble, original.CrossSection.Metrics)
				So(restored.CrossSection.index, ShouldResemble, map[string]int{
					"BTC/USD": 0, "ETH/USD": 1,
				})
				So(restored.Measurements, ShouldResemble, original.Measurements)
				So(restored.Forecasts, ShouldResemble, original.Forecasts)
				So(restored.Decisions, ShouldResemble, original.Decisions)
				So(restored.TradeJournal, ShouldResemble, original.TradeJournal)
				So(restored.Lifecycle, ShouldResemble, original.Lifecycle)
				So(restored.Findings, ShouldResemble, original.Findings)
				So(restored.Hypotheses, ShouldResemble, original.Hypotheses)
				So(restored.Categories, ShouldResemble, original.Categories)
				So(restored.Manifold, ShouldResemble, original.Manifold)
				So(restored.Resonance, ShouldResemble, original.Resonance)
				So(restored.Causal, ShouldResemble, original.Causal)
				So(restored.Graphs["BTC/USD"].Nodes().Len(), ShouldEqual, 3)
				So(restored.Graphs["BTC/USD"].Edges().Len(), ShouldEqual, 2)
			})

			Convey("Then the injected hub survives while runtime Signals are fresh", func() {
				_, exists := restored.Signals.Load("stale")
				So(restored.uiHub, ShouldEqual, (chan<- []byte)(uiHub))
				So(restored.Signals, ShouldNotBeNil)
				So(exists, ShouldBeFalse)
			})
		})

		Convey("When its schema version is unsupported", func() {
			record := &thesisRecord{}
			So(json.Unmarshal(payload, record), ShouldBeNil)
			record.SchemaVersion++
			corrupt, marshalErr := json.Marshal(record)
			So(marshalErr, ShouldBeNil)

			_, err := RestoreThesis(corrupt, nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unsupported Thesis record schema version")
		})

		Convey("When a graph node key no longer identifies its measurement", func() {
			record := &thesisRecord{}
			So(json.Unmarshal(payload, record), ShouldBeNil)
			record.Graphs[0].Nodes[0].Key = "corrupt"
			corrupt, marshalErr := json.Marshal(record)
			So(marshalErr, ShouldBeNil)

			_, err := RestoreThesis(corrupt, nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid node key")
		})

		Convey("When a graph edge references an absent node", func() {
			record := &thesisRecord{}
			So(json.Unmarshal(payload, record), ShouldBeNil)
			record.Graphs[0].Edges[0].To = "absent"
			corrupt, marshalErr := json.Marshal(record)
			So(marshalErr, ShouldBeNil)

			_, err := RestoreThesis(corrupt, nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid edge")
		})

		Convey("When cross-sectional recovery would choose between duplicates", func() {
			record := &thesisRecord{}
			So(json.Unmarshal(payload, record), ShouldBeNil)
			record.CrossSection.Metrics[1].Symbol = "BTC/USD"
			corrupt, marshalErr := json.Marshal(record)
			So(marshalErr, ShouldBeNil)

			_, err := RestoreThesis(corrupt, nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "duplicate symbol")
		})
	})
}

/*
BenchmarkThesisMarshalBinary measures deterministic serialization of a Thesis
carrying a realistically repeated measurement and relationship stream.
*/
func BenchmarkThesisMarshalBinary(b *testing.B) {
	fixture := thesisRecordFixture{at: time.Unix(100, 20).UTC()}
	thesis := fixture.thesis(b, 64)
	b.ReportAllocs()

	for b.Loop() {
		if _, err := thesis.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}

/*
BenchmarkRestoreThesis measures schema validation plus CrossSection and
Gonum topology reconstruction for a realistic durable lifecycle record.
*/
func BenchmarkRestoreThesis(b *testing.B) {
	fixture := thesisRecordFixture{at: time.Unix(100, 20).UTC()}
	payload, err := fixture.thesis(b, 64).MarshalBinary()

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := RestoreThesis(payload, nil); err != nil {
			b.Fatal(err)
		}
	}
}
