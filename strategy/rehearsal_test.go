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
	rehearsal.observations = map[string][]hindsight.Observation{}
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
func rehearsalEpisodes(rehearsal *Rehearsal) []hindsight.Episode {
	tape := market.NewOpportunityTape("BTC/USD", time.Unix(1700000000, 0), 6)
	observations := make([]hindsight.Observation, 0, len(tape.Steps)+3)
	for index, step := range tape.Steps {
		observations = append(observations, rehearsalObservation(uint64(index+1), 0,
			"BTC/USD", step.ExecutableBid, step.ExecutableBid+0.1, step.ExecutableBid+0.05))
	}
	anchor := observations[len(observations)-3]
	var episodes []hindsight.Episode
	for index, bid := range []float64{anchor.Ask * 1.1, anchor.Ask * 1.001, anchor.Bid * 0.9} {
		endpoint := rehearsalObservation(uint64(len(observations)+1), 0, "BTC/USD", bid, bid+0.1, bid+0.05)
		observations = append(observations, endpoint)
		kind, role := hindsight.EpisodeUpwardExcursion, hindsight.ReferencePeak
		if index == 2 {
			kind, role = hindsight.EpisodeDownwardExcursion, hindsight.ReferenceTrough
		}
		episodes = append(episodes, hindsight.Episode{Symbol: "BTC/USD", Kind: kind,
			HasObservedExcursion: true, ObservedExcursion: bid/anchor.Bid - 1, Confirmed: true,
			References: []hindsight.ReferencePoint{
				{Role: hindsight.ReferenceAnchor, Capture: anchor.Capture, Ordinal: anchor.Ordinal},
				{Role: role, Capture: endpoint.Capture, Ordinal: endpoint.Ordinal},
			},
		})
	}
	rehearsal.observations["BTC/USD"] = observations
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
		episodes := rehearsalEpisodes(rehearsal)
		pool, err := rehearsal.balance(episodes)
		So(err, ShouldBeNil)
		So(len(pool), ShouldBeGreaterThan, 0)

		for _, condition := range []string{"profitable", "friction", "declining", "illiquid", "quiet"} {
			Convey(condition+" is represented in the selected pool", func() {
				rehearsal := rehearsalFixture(t)
				episode := rehearsalEpisodes(rehearsal)[0]
				observations := rehearsal.observations[episode.Symbol]
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
				pool, err := rehearsal.balance([]hindsight.Episode{episode})
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

func TestNewRehearsal(t *testing.T) {
	Convey("Workers own independent historical models that do not clone the live trie", t, func() {
		learner, _ := learningFixture(t)
		rehearsal := learner.Rehearsal
		So(len(rehearsal.cursors), ShouldEqual, 2)
		So(rehearsal.cursors[0].columns, ShouldBeEmpty)
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
		So(rehearsal.Run(t.Context()), ShouldBeNil)
		rehearsal.Workload.Admit()
		for step := 0; step < len(tape.Steps)*4 && rehearsal.Wire().Passes < 2; step++ {
			rehearsal.Workload.Step(nil)
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
			first := &rehearsal.observations["BTC/USD"][0]
			count := len(rehearsal.observations["BTC/USD"])
			So(rehearsal.lastSequence, ShouldEqual, len(tape.Steps))
			So(rehearsal.Run(t.Context()), ShouldBeNil)
			So(rehearsal.cursors[0].learned == learned, ShouldBeTrue)
			So(&rehearsal.observations["BTC/USD"][0] == first, ShouldBeTrue)
			So(len(rehearsal.observations["BTC/USD"]), ShouldEqual, count)
			So(rehearsal.Wire().Passes, ShouldEqual, state.Passes)
			So(rehearsal.Wire().Trained, ShouldEqual, rehearsal.Wire().Decisions)
		})
	})
}

func TestRehearsalQuiet(t *testing.T) {
	Convey("Quiet practice comes from an unchanged interval closed by a real change", t, func() {
		rehearsal := rehearsalFixture(t)
		rehearsal.policy.Coordinate = hindsight.CoordinateMidpoint
		observations := rehearsalObservations()
		episodes := rehearsal.quiet("BTC/USD", observations)
		So(len(episodes), ShouldEqual, 1)
		So(episodes[0].Kind, ShouldEqual, hindsight.EpisodeQuiet)
		So(episodes[0].FromSequence, ShouldEqual, observations[0].Capture.Sequence)
		So(episodes[0].ToSequence, ShouldEqual, observations[2].Capture.Sequence)

		Convey("An unclosed quiet suffix supplies no completed episode", func() {
			So(rehearsal.quiet("BTC/USD", observations[7:]), ShouldBeEmpty)
		})
	})
}

func mustFragment(t testing.TB, rehearsal *Rehearsal, episode hindsight.Episode) []hindsight.Observation {
	t.Helper()
	fragment, err := rehearsal.prepare(episode)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
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
