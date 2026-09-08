package strategy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning/associative/model"
	"github.com/theapemachine/symm/tests/market"
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
		Domain:  "spot",
		Capture: hindsight.CaptureIdentity{Run: "rehearsal", Sequence: hindsight.CaptureSequence(sequence), Stream: "spot", StreamEpoch: 1},
		Ordinal: ordinal,
		Symbol:  symbol,
		Kind:    "ticker",
		VenueAt: at,
		HasBid:  bid > 0,
		Bid:     bid,
		HasAsk:  ask > 0,
		Ask:     ask,
		HasLast: last > 0,
		Last:    last,
	}
}

func TestGroupObservations(t *testing.T) {
	Convey("Observations are grouped, sorted by capture order and spot only", t, func() {
		grouped := groupObservations([]hindsight.Observation{
			rehearsalObservation(2, 0, "BTC/USD", 99, 101, 100),
			rehearsalObservation(1, 0, "BTC/USD", 98, 100, 99),
			rehearsalObservation(3, 0, "BTC/USD", 100, 102, 101),
			{Domain: "futures", Symbol: "BTC/USD", Capture: hindsight.CaptureIdentity{Run: "rehearsal", Sequence: 4, Stream: "futures", StreamEpoch: 1}},
		})

		So(len(grouped), ShouldEqual, 1)
		So(len(grouped["BTC/USD"]), ShouldEqual, 3)
		So(grouped["BTC/USD"][0].Capture.Sequence, ShouldEqual, hindsight.CaptureSequence(1))
		So(grouped["BTC/USD"][2].Capture.Sequence, ShouldEqual, hindsight.CaptureSequence(3))
	})
}

func rehearsalFixture() *Rehearsal {
	price := broker.NewPrice(nil, nil)
	// Fixture venue charges a quarter percent on each side of a round trip.
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.25)})
	return &Rehearsal{price: price, observations: map[string][]hindsight.Observation{},
		experiences: make(chan Experience, 1)}
}

func TestRehearsalGrade(t *testing.T) {
	Convey("Captured asks and bids determine the exercise return", t, func() {
		rehearsal := rehearsalFixture()
		anchor := rehearsalObservation(1, 0, "BTC/USD", 99, 101, 100)
		episode := hindsight.Episode{Symbol: "BTC/USD", HasObservedExcursion: true}

		Convey("Different price regimes separate opportunity, fee trap and decline", func() {
			for _, endpointBid := range []float64{110, 101, 90} {
				endpoint := rehearsalObservation(2, 0, "BTC/USD", endpointBid, endpointBid+2, endpointBid+1)
				entered, defined := rehearsal.grade(episode, Action{Kind: "enter"}, anchor, endpoint)
				expected := (endpointBid*0.9975 - 101*1.0025) / (101 * 1.0025)
				So(defined, ShouldBeTrue)
				So(entered, ShouldAlmostEqual, expected)
				waited, defined := rehearsal.grade(episode, Action{Kind: "wait"}, anchor, endpoint)
				So(defined, ShouldBeTrue)
				So(waited, ShouldAlmostEqual, -max(0, expected))
			}
		})

		Convey("Missing quotes and fees never become free trading", func() {
			endpoint := rehearsalObservation(2, 0, "BTC/USD", 110, 112, 111)
			anchor.HasAsk = false
			_, defined := rehearsal.grade(episode, Action{Kind: "enter"}, anchor, endpoint)
			So(defined, ShouldBeFalse)
			anchor.HasAsk = true
			episode.Symbol = "UNKNOWN/USD"
			_, defined = rehearsal.grade(episode, Action{Kind: "wait"}, anchor, endpoint)
			So(defined, ShouldBeFalse)
		})
	})
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
	return episodes
}

func TestRehearsalBalance(t *testing.T) {
	Convey("Every available class gets equal representation without fabricated episodes", t, func() {
		rehearsal := rehearsalFixture()
		episodes := rehearsalEpisodes(rehearsal)
		pool := rehearsal.balance(append(episodes, episodes[0], episodes[0]))
		So(len(pool), ShouldEqual, 3)
		So(rehearsal.Wire().Profitable, ShouldEqual, 3)
		So(rehearsal.Wire().Subfriction, ShouldEqual, 1)
		So(rehearsal.Wire().Declining, ShouldEqual, 1)
		Convey("An absent class stays visibly absent", func() {
			pool = rehearsal.balance(episodes[:1])
			So(len(pool), ShouldEqual, 1)
			So(rehearsal.Wire().Subfriction, ShouldEqual, 0)
			So(rehearsal.Wire().Declining, ShouldEqual, 0)
		})
		Convey("An unknown fee excludes and counts the entire pool", func() {
			rehearsal.price = broker.NewPrice(nil, nil)
			So(rehearsal.balance(episodes), ShouldBeEmpty)
			So(rehearsal.Wire().Ungraded, ShouldEqual, 3)
			So(rehearsal.Wire().Status, ShouldEqual, "waiting for gradeable episodes")
		})
	})
}

func TestRehearsalReplayEpisode(t *testing.T) {
	Convey("Workers learn from supported prefix context before publishing signed experience", t, func() {
		rehearsal := rehearsalFixture()
		episodes := rehearsalEpisodes(rehearsal)
		learned := model.New[string, Action]()
		var columns [][2]string
		So(rehearsal.replayEpisode(t.Context(), learned, episodes[0], &columns), ShouldBeNil)
		So(len(rehearsal.experiences), ShouldEqual, 1)
		experience := <-rehearsal.experiences
		So(experience.Authority, ShouldBeGreaterThan, 0)
		So(len(experience.Context), ShouldBeGreaterThan, 0)
		So(learned.Recall(experience.Symbol, experience.Context, experience.Action).Samples, ShouldEqual, 1)

		Convey("Changing the future quote changes the grade but not the context", func() {
			observations := rehearsal.observations["BTC/USD"]
			observations[len(observations)-3].Bid *= 0.5
			So(rehearsal.replayEpisode(t.Context(), learned, episodes[0], &columns), ShouldBeNil)
			changed := <-rehearsal.experiences
			So(changed.Context, ShouldResemble, experience.Context)
		})

		Convey("Cancellation cannot block on a full experience channel", func() {
			rehearsal.experiences <- experience
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			So(rehearsal.replayEpisode(ctx, learned, episodes[0], &columns), ShouldBeNil)
			So(len(rehearsal.experiences), ShouldEqual, 1)
		})
	})
}

func BenchmarkRehearsalReplayEpisode(b *testing.B) {
	rehearsal := rehearsalFixture()
	episodes := rehearsalEpisodes(rehearsal)
	learned := model.New[string, Action]()
	var columns [][2]string
	b.ReportAllocs()
	for b.Loop() {
		if err := rehearsal.replayEpisode(b.Context(), learned, episodes[0], &columns); err != nil {
			b.Fatal(err)
		}
		if len(rehearsal.experiences) != 1 {
			b.Fatal("missing replay experience")
		}
		<-rehearsal.experiences
	}
}

func TestNewRehearsal(t *testing.T) {
	Convey("Workers own independent historical models that do not clone the live trie", t, func() {
		learner, _ := learningFixture(t)
		rehearsal := learner.Rehearsal
		So(len(rehearsal.models), ShouldEqual, 2)
		So(len(rehearsal.columns), ShouldEqual, 2)
		So(rehearsal.models[0].Observe("BTC/USD", nil, Action{Kind: "enter"}, -0.01, 0.5), ShouldBeNil)
		So(rehearsal.models[1].Recall("BTC/USD", nil, Action{Kind: "enter"}).Defined, ShouldBeFalse)
		So(learner.Population.Agents[0].Model.Recall("BTC/USD", nil, Action{Kind: "enter"}).Defined, ShouldBeFalse)
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
				Bid:  decimal.NewFromFloat64(step.ExecutableBid),
				Ask:  decimal.NewFromFloat64(step.ExecutableBid + 0.1),
				Last: decimal.NewFromFloat64(step.ExecutableBid + 0.05),
			}}})
			So(err, ShouldBeNil)
			hash := sha256.Sum256(payload)
			writer.AddCapture(tables.CaptureRow{
				Run: string(learner.run), Sequence: int64(index + 1), Stream: "spot",
				StreamEpoch: 1, StreamSequence: int64(index + 1), ReceivedAt: step.EventTime,
				Kind: "ticker", PayloadHash: hex.EncodeToString(hash[:]), Payload: payload,
			})
		}
		So(writer.Commit(t.Context()), ShouldBeNil)
		So(rehearsal.Run(t.Context()), ShouldBeNil)
		state := rehearsal.Wire()
		So(state.Episodes, ShouldBeGreaterThan, 0)
		So(state.Passes, ShouldEqual, 1)
		So(state.Status, ShouldEqual, "pass complete")
		So(state.Trained, ShouldEqual, state.Decisions)
		So(state.Trained+state.Unsupported, ShouldEqual, state.PerWorker*uint64(state.Workers))

		Convey("The next pass reuses worker state and fully drains its experience", func() {
			learned := rehearsal.models[0]
			So(rehearsal.Run(t.Context()), ShouldBeNil)
			So(rehearsal.models[0] == learned, ShouldBeTrue)
			So(rehearsal.Wire().Passes, ShouldEqual, 2)
			So(rehearsal.Wire().Trained, ShouldEqual, rehearsal.Wire().Decisions)
		})
	})
}
