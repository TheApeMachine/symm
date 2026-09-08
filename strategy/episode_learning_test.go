package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/*
The tape is the teacher. These cover the loop that turns a confirmed excursion
into evidence the agent can act on — the loop whose absence meant the system
discovered every real mover and learned nothing at all from any of them.
*/

/*
episodeFixture builds one episode whose priced coordinates agree with the leg it
claims: the anchor at 100 and the extremum at 100*(1+excursion). The training
path reads those prices, never the net excursion field, so a fixture that
disagreed with itself would prove nothing.
*/
func episodeFixture(excursion float64, kind hindsight.EpisodeKind) hindsight.Episode {
	at := time.Unix(1000, 0)
	endpoint := hindsight.ReferencePeak

	if excursion < 0 {
		endpoint = hindsight.ReferenceTrough
	}

	return hindsight.Episode{
		ID: "episode-1", Symbol: "TEST/USD", Kind: kind, Confirmed: true,
		FromSequence: 100, ToSequence: 200,
		FromAt: at, ToAt: at.Add(2 * time.Minute),
		ObservedExcursion: excursion, HasObservedExcursion: true,
		References: []hindsight.ReferencePoint{
			{
				Role:    hindsight.ReferenceAnchor,
				Capture: hindsight.CaptureIdentity{Sequence: 100},
				VenueAt: at, Value: 100, HasValue: true,
			},
			{
				Role:    endpoint,
				Capture: hindsight.CaptureIdentity{Sequence: 200},
				VenueAt: at.Add(2 * time.Minute), Value: 100 * (1 + excursion), HasValue: true,
			},
			{
				Role:    hindsight.ReferenceReversal,
				Capture: hindsight.CaptureIdentity{Sequence: 260},
				VenueAt: at.Add(3 * time.Minute), Value: 100 * (1 + excursion/2), HasValue: true,
			},
		},
	}
}

/*
reversalFixture is the episode kind that carries a whole trade: a run up into a
turning point, then the give-back. Its net excursion is near zero — it ends
close to where it began — while the leg into the turn was seventeen percent.
Training must read the leg, never the net.
*/
func reversalFixture() hindsight.Episode {
	at := time.Unix(1000, 0)

	return hindsight.Episode{
		ID: "reversal-1", Symbol: "TEST/USD", Kind: hindsight.EpisodeReversal, Confirmed: true,
		FromSequence: 100, ToSequence: 260,
		FromAt: at, ToAt: at.Add(3 * time.Minute),
		ObservedExcursion: 0.001, HasObservedExcursion: true,
		Traversed: 0.34, HasTraversed: true,
		References: []hindsight.ReferencePoint{
			{
				Role:    hindsight.ReferenceAnchor,
				Capture: hindsight.CaptureIdentity{Sequence: 100},
				VenueAt: at, Value: 100, HasValue: true,
			},
			{
				Role:    hindsight.ReferenceReversal,
				Capture: hindsight.CaptureIdentity{Sequence: 190},
				VenueAt: at.Add(2 * time.Minute), Value: 117, HasValue: true,
			},
			{
				Role:    hindsight.ReferenceExitAnchor,
				Capture: hindsight.CaptureIdentity{Sequence: 260},
				VenueAt: at.Add(3 * time.Minute), Value: 100.1, HasValue: true,
			},
		},
	}
}

/* reviewerFixture wires a market whose trail already holds two known contexts. */
func reviewerFixture() (*PolicyReview, *learningMarket, *Knowledge) {
	knowledge := NewKnowledge(learning.NewGrid())
	local := &LocalLearning{Knowledge: knowledge, markets: map[string]*learningMarket{}}
	market := &learningMarket{symbol: "TEST/USD", cost: 0.016}
	market.trail = []observedContext{
		{seq: 90, at: time.Unix(990, 0), accountState: "flat", context: []uint64{11, FrameDelimiter, 12}},
		{seq: 190, at: time.Unix(1100, 0), accountState: "holding", context: []uint64{21, FrameDelimiter, 22}},
	}
	local.markets["TEST/USD"] = market

	return &PolicyReview{local: local}, market, knowledge
}

func TestEpisodeTraining(t *testing.T) {
	Convey("A confirmed excursion becomes evidence, not just a tally", t, func() {
		reviewer, market, knowledge := reviewerFixture()
		episode := episodeFixture(0.40, hindsight.EpisodeUpwardExcursion)

		trained, reason := reviewer.train(market, episode)
		So(reason, ShouldBeBlank)
		So(trained, ShouldBeTrue)

		Convey("Entering where the move began is now the supported action", func() {
			enter := knowledge.Reading("TEST/USD", "flat", []uint64{11, FrameDelimiter, 12},
				LearningAction{Kind: types.ActionEnter})
			hold := knowledge.Reading("TEST/USD", "flat", []uint64{11, FrameDelimiter, 12},
				LearningAction{Kind: types.ActionHold})

			So(enter.Economic.Defined, ShouldBeTrue)
			So(hold.Economic.Defined, ShouldBeTrue)
			So(enter.Economic.Rate, ShouldBeGreaterThan, hold.Economic.Rate)
		})

		/*
			A 40% move that costs 1.6% to get in and out of is evidence about a
			38% move. Stating it net is what keeps the fee-dominated economics
			inside the evidence rather than in a caveat beside it.
		*/
		Convey("The evidence is net of the round trip the trade would have paid", func() {
			enter := knowledge.Reading("TEST/USD", "flat", []uint64{11, FrameDelimiter, 12},
				LearningAction{Kind: types.ActionEnter})
			So(enter.Economic.GrowthMean, ShouldAlmostEqual, 0.3365-0.016, 0.002)
		})

		Convey("Leaving where it exhausted beats holding past it", func() {
			exit := knowledge.Reading("TEST/USD", "holding", []uint64{21, FrameDelimiter, 22},
				LearningAction{Kind: types.ActionExit, Reduce: true})
			hold := knowledge.Reading("TEST/USD", "holding", []uint64{21, FrameDelimiter, 22},
				LearningAction{Kind: types.ActionHold})

			So(exit.Economic.Defined, ShouldBeTrue)
			So(hold.Economic.Defined, ShouldBeTrue)
			So(hold.Economic.GrowthMean, ShouldBeLessThan, 0)
			So(exit.Economic.Rate, ShouldBeGreaterThan, hold.Economic.Rate)
		})
	})

	/*
		The desk holds spot inventory. A fall is not an entry it could have
		taken, so it must never be trained as one.
	*/
	Convey("A downward excursion teaches that holding through it was the mistake", t, func() {
		reviewer, market, knowledge := reviewerFixture()

		trained, reason := reviewer.train(market, episodeFixture(-0.30, hindsight.EpisodeDownwardExcursion))
		So(reason, ShouldBeBlank)
		So(trained, ShouldBeTrue)

		context := []uint64{11, FrameDelimiter, 12}
		hold := knowledge.Reading("TEST/USD", "flat", context, LearningAction{Kind: types.ActionHold})
		enter := knowledge.Reading("TEST/USD", "flat", context, LearningAction{Kind: types.ActionEnter})

		So(hold.Economic.Defined, ShouldBeTrue)
		So(hold.Economic.GrowthMean, ShouldBeLessThan, 0)
		So(enter.Economic.Defined, ShouldBeFalse)

		/*
			The desk was flat here, and a flat desk cannot exit. Crediting an
			exit on this context would put weight on an action that was never in
			its feasible set, and every later recall would read it as a real
			alternative to waiting.
		*/
		Convey("Exiting is not credited to a desk that held nothing", func() {
			So(knowledge.Reading("TEST/USD", "flat", context,
				LearningAction{Kind: types.ActionExit, Reduce: true}).Economic.Defined, ShouldBeFalse)
		})

		Convey("A desk that was holding does learn that leaving was worth it", func() {
			reviewer, market, knowledge := reviewerFixture()
			market.trail[0].accountState = "holding"

			trained, _ := reviewer.train(market, episodeFixture(-0.30, hindsight.EpisodeDownwardExcursion))
			So(trained, ShouldBeTrue)

			exit := knowledge.Reading("TEST/USD", "holding", context,
				LearningAction{Kind: types.ActionExit, Reduce: true})
			hold := knowledge.Reading("TEST/USD", "holding", context,
				LearningAction{Kind: types.ActionHold})

			So(exit.Economic.Defined, ShouldBeTrue)
			So(exit.Economic.Rate, ShouldBeGreaterThan, hold.Economic.Rate)
		})
	})

	Convey("Evidence the agent cannot honestly recover is refused, not invented", t, func() {
		reviewer, market, knowledge := reviewerFixture()

		Convey("An episode older than the retained trail trains nothing", func() {
			market.trail = market.trail[1:]
			trained, reason := reviewer.train(market, episodeFixture(0.40, hindsight.EpisodeUpwardExcursion))

			So(trained, ShouldBeFalse)
			So(reason, ShouldContainSubstring, "no longer retained")
			So(knowledge.Reading("TEST/USD", "flat", []uint64{11, FrameDelimiter, 12},
				LearningAction{Kind: types.ActionEnter}).Economic.Defined, ShouldBeFalse)
		})

		Convey("An episode without an anchor trains nothing", func() {
			episode := episodeFixture(0.40, hindsight.EpisodeUpwardExcursion)
			episode.References = episode.References[1:]
			trained, reason := reviewer.train(market, episode)

			So(trained, ShouldBeFalse)
			So(reason, ShouldEqual, "no anchor reference")
		})

		Convey("An episode with no priced coordinates trains nothing", func() {
			episode := episodeFixture(0.40, hindsight.EpisodeUpwardExcursion)
			episode.References[1].HasValue = false
			trained, reason := reviewer.train(market, episode)

			So(trained, ShouldBeFalse)
			So(reason, ShouldEqual, "no priced anchor and extremum")
		})

		/*
			The net excursion field is not what is read. A reversal reports a
			net near zero while its leg covered seventeen percent, so a trainer
			that read the net would learn that nothing happened.
		*/
		Convey("A reversal is trained on its leg, not on its near-zero net", func() {
			reviewer, market, knowledge := reviewerFixture()
			episode := reversalFixture()

			trained, reason := reviewer.train(market, episode)
			So(reason, ShouldBeBlank)
			So(trained, ShouldBeTrue)

			enter := knowledge.Reading("TEST/USD", "flat", []uint64{11, FrameDelimiter, 12},
				LearningAction{Kind: types.ActionEnter})

			So(enter.Economic.Defined, ShouldBeTrue)
			So(enter.Economic.GrowthMean, ShouldAlmostEqual, math.Log(1.17)-0.016, 0.002)
			So(enter.Economic.GrowthMean, ShouldBeGreaterThan, 0.1)
		})

		Convey("Holding past the turn is trained on the give-back it cost", func() {
			reviewer, market, knowledge := reviewerFixture()

			trained, _ := reviewer.train(market, reversalFixture())
			So(trained, ShouldBeTrue)

			hold := knowledge.Reading("TEST/USD", "holding", []uint64{21, FrameDelimiter, 22},
				LearningAction{Kind: types.ActionHold})

			So(hold.Economic.Defined, ShouldBeTrue)
			So(hold.Economic.GrowthMean, ShouldAlmostEqual, math.Log(100.1/117), 0.002)
		})
	})
}

func TestObservedContextTrail(t *testing.T) {
	Convey("The agent retains what it conditioned on, addressed by capture", t, func() {
		market := &learningMarket{symbol: "TEST/USD"}

		for step := range 5 {
			market.seq = hindsight.CaptureSequence(10 * (step + 1))
			market.at = time.Unix(int64(1000+step), 0)
			market.observeContext([]uint64{uint64(step + 1)}, "flat")
		}

		So(market.trail, ShouldHaveLength, 5)

		Convey("An unchanged context is one entry, not one per book update", func() {
			market.seq = 60
			market.observeContext([]uint64{5}, "flat")
			So(market.trail, ShouldHaveLength, 5)
		})

		Convey("A coordinate resolves to the context in force at or before it", func() {
			found, known := market.contextAt(35)
			So(known, ShouldBeTrue)
			So(found.context, ShouldResemble, []uint64{3})
		})

		Convey("A coordinate older than the trail is unknown, not the oldest entry", func() {
			_, known := market.contextAt(5)
			So(known, ShouldBeFalse)
		})
	})
}

func TestEpisodeWarmupFromRetainedRecord(t *testing.T) {
	Convey("Excursions on the retained record teach the contexts that preceded them", t, func() {
		knowledge := NewKnowledge(learning.NewGrid())
		agent := &Agent{
			LocalLearning: &LocalLearning{Knowledge: knowledge, markets: map[string]*learningMarket{}},
			PolicyReview:  &PolicyReview{},
		}
		at := time.Unix(1000, 0)
		contexts := []hindsight.LearningEvent{
			{
				Symbol: "TEST/USD", Kind: "issued", Mode: "policy", At: at,
				Capture: hindsight.CaptureIdentity{Sequence: 90},
				Context: []uint64{11, FrameDelimiter, 12}, Inventory: "0",
			},
			{
				Symbol: "TEST/USD", Kind: "issued", Mode: "policy", At: at.Add(time.Minute),
				Capture: hindsight.CaptureIdentity{Sequence: 190},
				Context: []uint64{21, FrameDelimiter, 22}, Inventory: "5",
			},
		}
		fees := func(string) float64 { return 0.016 }

		report := agent.WarmupEpisodes(
			[]hindsight.Episode{episodeFixture(0.40, hindsight.EpisodeUpwardExcursion)},
			contexts, fees,
		)

		So(report.Episodes, ShouldEqual, 1)
		So(report.Trained, ShouldEqual, 1)
		So(report.Uncontexted, ShouldEqual, 0)

		Convey("The agent now prefers entering where that move began", func() {
			context := []uint64{11, FrameDelimiter, 12}
			enter := knowledge.Reading("TEST/USD", "flat", context, LearningAction{Kind: types.ActionEnter})
			hold := knowledge.Reading("TEST/USD", "flat", context, LearningAction{Kind: types.ActionHold})

			So(enter.Economic.Defined, ShouldBeTrue)
			So(enter.Economic.Rate, ShouldBeGreaterThan, hold.Economic.Rate)
		})

		/*
			The account state at the coordinate is the one the journal recorded,
			so evidence about entering never lands on the scope for a desk that
			was already holding.
		*/
		Convey("The exit is trained on the state the desk was actually in", func() {
			context := []uint64{21, FrameDelimiter, 22}
			So(knowledge.Reading("TEST/USD", "holding", context,
				LearningAction{Kind: types.ActionExit, Reduce: true}).Economic.Defined, ShouldBeTrue)
			So(knowledge.Reading("TEST/USD", "flat", context,
				LearningAction{Kind: types.ActionExit, Reduce: true}).Economic.Defined, ShouldBeFalse)
		})

		Convey("An episode with no journalled context before it is reported, not guessed", func() {
			report := agent.WarmupEpisodes(
				[]hindsight.Episode{episodeFixture(0.40, hindsight.EpisodeUpwardExcursion)},
				contexts[1:], fees,
			)

			So(report.Trained, ShouldEqual, 0)
			So(report.Uncontexted, ShouldEqual, 1)
		})

		Convey("An unconfirmed episode is never trained on", func() {
			episode := episodeFixture(0.40, hindsight.EpisodeUpwardExcursion)
			episode.Confirmed = false
			report := agent.WarmupEpisodes([]hindsight.Episode{episode}, contexts, fees)

			So(report.Episodes, ShouldEqual, 0)
			So(report.Trained, ShouldEqual, 0)
		})
	})
}
