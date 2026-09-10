package strategy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/types"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func rehearsalObservation(
	sequence, ordinal uint64,
	symbol string,
	bid, ask, last float64,
) hindsight.Observation {
	at := time.Unix(1700000000, 0).Add(time.Duration(sequence) * time.Second)

	return hindsight.Observation{
		Domain:     "spot",
		Capture:    hindsight.CaptureIdentity{Run: "rehearsal", Sequence: hindsight.CaptureSequence(sequence), Stream: "spot", StreamEpoch: 1},
		Ordinal:    ordinal,
		Symbol:     symbol,
		Kind:       "ticker",
		VenueAt:    at,
		ReceivedAt: at,
		BidQty:     1, AskQty: 1,
		HasBid:  bid > 0,
		Bid:     bid,
		HasAsk:  ask > 0,
		Ask:     ask,
		HasLast: last > 0,
		Last:    last,
	}
}

// rehearsalEnvelope supplies a captured producer measurement whose changing
// order-flow coordinates are available at that event, independent of labels.
func rehearsalEnvelope(observation hindsight.Observation) *types.Envelope {
	measurement := data.NewMeasurement[float64]("captured-depth", observation.Symbol, "depthflow", observation.ReceivedAt, time.Unix(1700000000, 0))
	measurement.Maturity = 1
	measurement.PutMetric(data.Metric[float64]{Label: "observed_notional", Raw: observation.Bid * observation.BidQty})
	measurement.PutMetric(data.Metric[float64]{Label: "ask_notional", Raw: observation.Ask * observation.AskQty})
	measurement.SNR, measurement.SNRDefined = 100, true
	return &types.Envelope{Key: observation.Symbol, CaptureID: observation.Capture, CaptureOrdinal: observation.Ordinal, DepthFlow: measurement}
}

func rehearsalFixture(t testing.TB) *Rehearsal {
	t.Helper()
	learner, _ := learningFixture(t)
	rehearsal := learner.Rehearsal
	rehearsal.observations = map[tapeKey][]hindsight.Observation{}
	rehearsal.inputs = make(map[hindsight.EnvelopeRef]hindsight.RehearsalInput)
	return rehearsal
}

func practicePrice() *broker.Price {
	price := broker.NewPrice(nil, nil)
	// This fixture charges a quarter percent on each side of a round trip.
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(.25)})
	return price
}

// The market fixture supplies a changing multi-leg causal prefix. Endpoints
// below deliberately exercise profitable, sub-friction and declining quotes.
/* practiceTapeKey names the fixture's tape inside the fixture's own run. */
func practiceTapeKey(rehearsal *Rehearsal, symbol string) tapeKey {
	return tapeKey{run: rehearsal.run, symbol: symbol}
}

func rehearsalEpisodes(rehearsal *Rehearsal, legs int) []discovered {
	tape := market.NewOpportunityTape("BTC/USD", time.Unix(1700000000, 0), legs)
	observations := make([]hindsight.Observation, 0, len(tape.Steps)+3)
	for index, step := range tape.Steps {
		observations = append(observations, rehearsalObservation(uint64(index+1), 0,
			"BTC/USD", step.ExecutableBid, step.ExecutableBid+0.1, step.ExecutableBid+0.05))
	}
	anchor := observations[len(observations)-3]
	var episodes []discovered
	for index, bid := range []float64{anchor.Ask * 1.1, anchor.Ask * 1.001, anchor.Bid * 0.9} {
		endpoint := rehearsalObservation(uint64(len(observations)+1), 0, "BTC/USD", bid, bid+0.1, bid+0.05)
		observations = append(observations, endpoint)
		kind, role := hindsight.EpisodeUpwardExcursion, hindsight.ReferencePeak
		if index == 2 {
			kind, role = hindsight.EpisodeDownwardExcursion, hindsight.ReferenceTrough
		}
		episodes = append(episodes, discovered{
			tape: practiceTapeKey(rehearsal, "BTC/USD"),
			episode: hindsight.Episode{Symbol: "BTC/USD", Kind: kind,
				HasObservedExcursion: true, ObservedExcursion: bid/anchor.Bid - 1, Confirmed: true,
				References: []hindsight.ReferencePoint{
					{Role: hindsight.ReferenceAnchor, Capture: anchor.Capture, Ordinal: anchor.Ordinal},
					{Role: role, Capture: endpoint.Capture, Ordinal: endpoint.Ordinal},
				},
			},
		})
	}
	rehearsal.observations[practiceTapeKey(rehearsal, "BTC/USD")] = observations
	for _, observation := range observations {
		envelope := rehearsalEnvelope(observation)
		reference := hindsight.EnvelopeRef{Origin: observation.Capture, Ordinal: observation.Ordinal}
		input, err := (hindsight.ArtifactWitness{Envelope: reference, Payload: envelope.EncodeBytes()}).RehearsalInput()
		if err != nil {
			panic(err)
		}
		rehearsal.inputs[reference] = input
	}
	return episodes
}

func TestRehearsalBalance(t *testing.T) {
	Convey("Complete mini tapes are selected with both prefix and tail", t, func() {
		rehearsal := rehearsalFixture(t)
		episodes := rehearsalEpisodes(rehearsal, 6)
		pool, err := rehearsal.balance(episodes)
		So(err, ShouldBeNil)
		So(len(pool), ShouldBeGreaterThan, 0)

		for _, condition := range []string{"profitable", "friction", "declining", "illiquid", "quiet"} {
			Convey(condition+" is represented in the selected pool", func() {
				rehearsal := rehearsalFixture(t)
				found := rehearsalEpisodes(rehearsal, 6)[0]
				episode := found.episode
				observations := rehearsal.observations[found.tape]
				anchor := indexOfObservation(observations, episode, hindsight.ReferenceAnchor)
				endpoint := indexOfObservation(observations, episode, hindsight.ReferencePeak)

				switch condition {
				case "friction":
					observations[endpoint].Bid = observations[anchor].Ask * 1.001
				case "declining":
					observations[endpoint].Bid = observations[anchor].Bid * .9
					episode.ObservedExcursion = -.1
				case "illiquid":
					observations[endpoint].BidQty = 0
				case "quiet":
					observations[endpoint].Bid = observations[anchor].Bid
					episode.Kind = hindsight.EpisodeQuiet
				}
				found.episode = episode
				pool, err := rehearsal.balance([]discovered{found})
				So(err, ShouldBeNil)
				So(len(pool), ShouldEqual, 1)
				counts := map[string]uint64{
					"profitable": rehearsal.progress.Profitable,
					"friction":   rehearsal.progress.Subfriction,
					"declining":  rehearsal.progress.Declining,
					"illiquid":   rehearsal.progress.Illiquid,
					"quiet":      rehearsal.progress.Quiet,
				}
				So(counts[condition], ShouldEqual, 1)
			})
		}

		Convey("Missing fees fail explicitly", func() {
			rehearsal.price = broker.NewPrice(nil, nil)
			_, err := rehearsal.balance(episodes)
			So(err, ShouldNotBeNil)
		})
	})
}

/*
Stillness is abundant and movement is rare, so the share each class contributes
decides what the workers actually practise. Sizing it by the smallest present
class let one scarce class discard every other class's tapes; sizing it by the
largest handed the entire curriculum to stillness.
*/
func TestRehearsalPoolShare(t *testing.T) {
	Convey("No class can flood the pool and none can starve it", t, func() {
		share := func(counts ...int) []int {
			var classes [5][]selected

			for class, count := range counts {
				classes[class] = make([]selected, count)
			}
			perClass := poolShare(classes)
			taken := make([]int, len(counts))

			for class, episodes := range classes[:len(counts)] {
				taken[class] = min(perClass, len(episodes))
			}

			return taken
		}

		// The shape that was actually observed: one rare move against
		// thousands of still spans.
		So(share(2, 1, 3, 1, 5000), ShouldResemble, []int{2, 1, 2, 1, 2})

		// A single scarce class no longer throws the rest of the pool away.
		So(share(40, 40, 40, 1, 40), ShouldResemble, []int{40, 40, 40, 1, 40})

		// One present class is the whole pool, and takes all of itself.
		So(share(0, 0, 0, 0, 12), ShouldResemble, []int{0, 0, 0, 0, 12})
		So(share(0, 0, 0, 0, 0), ShouldResemble, []int{0, 0, 0, 0, 0})
	})
}

func TestNewRehearsal(t *testing.T) {
	Convey("Workers own independent historical models that do not clone the live trie", t, func() {
		learner, _ := learningFixture(t)
		rehearsal := learner.Rehearsal
		So(len(rehearsal.cursors), ShouldEqual, 2)
		So(rehearsal.cursors[0].space.Columns, ShouldBeEmpty)
		rehearsal.cursors[0].learned.Observe(agent.ContextKey("BTC/USD", nil), []byte(fmt.Sprint(Action{Kind: "enter"})), -0.01*0.5)
		So(rehearsal.cursors[1].learned.Evaluate(agent.ContextKey("BTC/USD", nil)).WinnerClass, ShouldBeEmpty)
		So(learner.Agent.Model.Evaluate(agent.ContextKey("BTC/USD", nil)).WinnerClass, ShouldBeEmpty)
	})
}

func TestRehearsalRun(t *testing.T) {
	Convey("Stored multi-leg ticker bytes run through discovery, workers and consolidation", t, func() {
		learner, _ := learningFixture(t)
		rehearsal := learner.Rehearsal
		// The fixture moves in discrete single-quote jumps separated by flat
		// quotes. Its selector uses one-quote dispersion to expose those legs.
		rehearsal.policy.ExcursionSigmas = 1
		rehearsal.policy.ExcursionHorizon = 1
		tape := market.NewOpportunityTape("BTC/USD", time.Unix(1700000000, 0), 32)
		writer := tables.NewWriter(learner.catalog)

		for index, step := range tape.Steps {
			payload, err := json.Marshal(kraken.Ticker{Data: []kraken.TickerData{{
				Symbol: "BTC/USD", Timestamp: step.EventTime,
				Bid:    decimal.NewFromFloat64(step.ExecutableBid),
				Ask:    decimal.NewFromFloat64(step.ExecutableBid + 0.1),
				Last:   decimal.NewFromFloat64(step.ExecutableBid + 0.05),
				BidQty: 1, AskQty: 1,
			}}})
			So(err, ShouldBeNil)
			observation := rehearsalObservation(uint64(index+1), 0, "BTC/USD", step.ExecutableBid, step.ExecutableBid+0.1, step.ExecutableBid+0.05)
			observation.Capture.Run = learner.run
			observation.ReceivedAt, observation.VenueAt = step.EventTime, step.EventTime
			writer.AddWitness(tables.WitnessRow{Run: string(learner.run), Envelope: tables.EnvelopeRefRow{Run: string(learner.run), Sequence: int64(index + 1)}, ArtifactKind: "precursor", Payload: rehearsalEnvelope(observation).EncodePrecursor()})
			hash := sha256.Sum256(payload)
			writer.AddCapture(tables.CaptureRow{
				Run: string(learner.run), Sequence: int64(index + 1), Stream: "spot",
				StreamEpoch: 1, StreamSequence: int64(index + 1), ReceivedAt: step.EventTime,
				Kind: "ticker", PayloadHash: hex.EncodeToString(hash[:]), Payload: payload,
			})
		}
		So(writer.Commit(t.Context()), ShouldBeNil)
		Convey("A later archive failure cannot withhold already prepared fragments", func() {
			writer.AddRun(tables.RunRow{ID: "older", StartedAt: time.Unix(1, 0)})
			writer.AddCapture(tables.CaptureRow{Run: "older", Sequence: 1,
				Kind: "ticker", Payload: []byte("malformed")})
			So(writer.Commit(t.Context()), ShouldBeNil)
			So(rehearsal.Run(t.Context()), ShouldNotBeNil)
			So(rehearsal.Wire().Observations, ShouldBeGreaterThan, 0)

			for _, cursor := range rehearsal.cursors {
				So(len(cursor.pending), ShouldBeGreaterThan, 0)
			}
		})

		Convey("Completed reads feed replay and subsequent passes retain their state", func() {

			So(rehearsal.Run(t.Context()), ShouldBeNil)
			rehearsal.Workload.Admit()
			// Two complete traversals include every fixture fragment despite
			// shuffling. Two short completed fragments alone do not prove that
			// a worker has seen the rich tape needed to form its grid.
			traversal := 0

			for _, cursor := range rehearsal.cursors {
				length := 0

				for _, fragment := range cursor.pending {
					length += len(fragment.observations)
				}
				traversal = max(traversal, length)
			}

			for step := 0; step < traversal*2; step++ {
				rehearsal.Workload.Step(nil)
				state := rehearsal.Wire()

				if state.Passes >= 2 && state.Trained > 0 {
					break
				}
			}
			So(rehearsal.Error(), ShouldBeNil)
			state := rehearsal.Wire()
			So(state.Episodes, ShouldBeGreaterThan, 0)
			So(state.Passes, ShouldBeGreaterThanOrEqualTo, 2)

			So(state.Trained, ShouldEqual, state.Decisions)
			So(state.Trained, ShouldBeGreaterThan, 0)

			for _, input := range rehearsal.inputs {
				for _, measurement := range input.Measurements {
					So(measurement.Metrics["observed_notional"].Coordinates, ShouldBeNil)
				}
			}
			So(state.Trained+state.Unsupported, ShouldBeGreaterThan, 0)

			Convey("The next pass reuses worker state and fully drains its experience", func() {
				learned := rehearsal.cursors[0].learned
				first := &rehearsal.observations[practiceTapeKey(rehearsal, "BTC/USD")][0]
				count := len(rehearsal.observations[practiceTapeKey(rehearsal, "BTC/USD")])
				So(rehearsal.sequences[rehearsal.run], ShouldEqual, len(tape.Steps))
				So(rehearsal.Run(t.Context()), ShouldBeNil)
				So(rehearsal.cursors[0].learned == learned, ShouldBeTrue)
				So(&rehearsal.observations[practiceTapeKey(rehearsal, "BTC/USD")][0] == first, ShouldBeTrue)
				So(len(rehearsal.observations[practiceTapeKey(rehearsal, "BTC/USD")]), ShouldEqual, count)
				So(rehearsal.Wire().Passes, ShouldEqual, state.Passes)
				So(rehearsal.Wire().Trained, ShouldEqual, rehearsal.Wire().Decisions)
			})
		})
	})
}

func TestRehearsalQuiet(t *testing.T) {
	Convey("Quiet practice comes from an unchanged interval closed by a real change", t, func() {
		rehearsal := rehearsalFixture(t)
		rehearsal.policy.Coordinate = hindsight.CoordinateMidpoint
		span := rehearsal.policy.MinRegimeSpan

		/*
			still builds an unchanged run of the declared length, closed by one
			observation at a different price. Stillness is what the record has
			most of, so the interesting question is exactly where the bar for
			calling it an episode sits.
		*/
		still := func(length int) []hindsight.Observation {
			observations := make([]hindsight.Observation, 0, length+1)

			for index := range length {
				observations = append(observations, rehearsalObservation(
					uint64(index+1), 0, "BTC/USD", 99, 100, 99.5))
			}

			return append(observations, rehearsalObservation(
				uint64(length+1), 0, "BTC/USD", 199, 200, 199.5))
		}
		observations := still(span + 1)
		episodes := rehearsal.quiet("BTC/USD", observations)
		So(len(episodes), ShouldEqual, 1)
		So(episodes[0].Kind, ShouldEqual, hindsight.EpisodeQuiet)
		So(episodes[0].FromSequence, ShouldEqual, observations[0].Capture.Sequence)
		So(episodes[0].ToSequence, ShouldEqual, observations[span].Capture.Sequence)

		/*
			A coordinate that simply was not requoted for a few observations is
			not evidence of stillness. Without this bar the midpoint's own
			quantisation manufactured an episode between almost every pair of
			observations, and stillness took the whole pool on volume alone.
		*/
		Convey("A span shorter than the policy's regime span is not an episode", func() {
			So(rehearsal.quiet("BTC/USD", still(span)), ShouldBeEmpty)
			So(rehearsal.quiet("BTC/USD", still(2)), ShouldBeEmpty)
		})

		Convey("An unclosed quiet suffix supplies no completed episode", func() {
			So(rehearsal.quiet("BTC/USD", observations[:span+1]), ShouldBeEmpty)
		})
	})
}

func mustFragment(t testing.TB, rehearsal *Rehearsal, found discovered) fragment {
	t.Helper()
	tape, err := rehearsal.prepare(found.tape, found.episode)
	if err != nil {
		t.Fatal(err)
	}
	return tape
}

func rehearsalObservations() []hindsight.Observation {
	tape := market.NewPrecursorTape("BTC/USD", time.Unix(1700000000, 0))
	observations := make([]hindsight.Observation, len(tape.Steps))

	for index, step := range tape.Steps {
		observations[index] = rehearsalObservation(uint64(index+1), 0, tape.Symbol,
			step.ExecutableBid, step.ExecutableBid+0.1, step.ExecutableBid+0.05)
	}
	return observations
}

func BenchmarkRehearsalDistribute(b *testing.B) {
	rehearsal := rehearsalFixture(b)
	// This fixture's single-quote jumps use the same discovery selector as
	// TestRehearsalRun so the benchmark exercises actual fragment delivery.
	rehearsal.policy.ExcursionSigmas = 1
	rehearsal.policy.ExcursionHorizon = 1
	rehearsalEpisodes(rehearsal, 32)
	b.ReportAllocs()

	for b.Loop() {
		clear(rehearsal.prepared)

		for _, cursor := range rehearsal.cursors {
			cursor.pending = cursor.pending[:0]
		}

		if err := rehearsal.distribute(); err != nil {
			b.Fatal(err)
		}

		if len(rehearsal.cursors[0].pending) == 0 {
			b.Fatal("no historical fragments reached the worker")
		}
	}
}
