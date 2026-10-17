package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
frameSink keeps every Value the Sequitur io.Copy's to it, as the hub would
receive them.
*/
type frameSink struct {
	values    []*data.Value
	cognition []*data.Value
}

func (sink *frameSink) Write(p []byte) (int, error) {
	value := data.New()

	if _, err := value.Write(p); err != nil {
		return 0, err
	}

	if value.Origin == data.CognitionOrigin {
		sink.cognition = append(sink.cognition, value)
		return len(p), nil
	}

	sink.values = append(sink.values, value)
	return len(p), nil
}

/*
gridTick is one stored-shape tick for symbols, each carrying the given
pinned signal metrics.
*/
func gridTick(tick uint64, metrics map[string]map[string]float64, symbols ...string) *data.Value {
	trades := make([]kraken.TradeData, 0, len(symbols))

	for idx, symbol := range symbols {
		trades = append(trades, kraken.TradeData{
			Symbol: symbol, Side: "buy", Price: *decimal.NewFromFloat64(100), Qty: 0.1,
			OrderType: "market", TradeID: int64(tick*10) + int64(idx), Timestamp: time.Unix(int64(tick), 0).UTC(),
		})
	}

	frame, err := sonic.Marshal(map[string]any{"channel": "trade", "type": "update", "data": trades})
	So(err, ShouldBeNil)

	value := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
	value.Epoch, value.Tick, value.Timestamp = 7, tick, tick*uint64(time.Second)
	value.FormatPrefix()
	value.AppendJSON(frame, data.TradeIndex("update"))

	for _, symbol := range symbols {
		price, err := sonic.Marshal(broker.PriceLayer{
			Symbol: symbol, Bid: decimal.NewFromFloat64(99.9), Ask: decimal.NewFromFloat64(100.1),
		})
		So(err, ShouldBeNil)
		value.AppendJSON(price, data.SymbolIndex(data.LayerPrice, symbol))

		for source, labels := range metrics {
			for label, metric := range labels {
				data.AppendSignalVector(value, source, symbol, []float64{metric}, label)
			}
		}
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, value)
	So(err, ShouldBeNil)

	return value
}

func TestGrammarTrace(t *testing.T) {
	Convey("Given a traced grammar", t, func() {
		grammar := NewGrammar().Trace()

		Convey("abcdbcabcd creates, reuses and inlines rules, in order", func() {
			for _, token := range []string{"a", "b", "c", "d", "b", "c", "a", "b", "c", "d"} {
				grammar.Append(token)
			}

			events := grammar.Drain()
			kinds := make([]string, 0, len(events))
			appends := 0

			for _, event := range events {
				kinds = append(kinds, event.Kind)

				if event.Kind == EventAppend {
					appends++
				}
			}

			So(appends, ShouldEqual, 10)
			So(events[5], ShouldResemble, GrammarEvent{Kind: EventAppend, Token: "c"})
			So(events[6], ShouldResemble, GrammarEvent{Kind: EventCreate, Rule: 1, Into: 0, From: 0, Digram: [2]string{"b", "c"}})
			So(kinds, ShouldContain, EventReuse)
			So(kinds, ShouldContain, EventInline)
			So(grammar.Drain(), ShouldBeEmpty)
			So(grammar.Check(), ShouldBeEmpty)
			So(grammar.StartRule().Terminals(), ShouldHaveLength, 10)
		})

		Convey("An untraced grammar records nothing", func() {
			plain := NewGrammar()
			plain.Append("a")
			plain.Append("a")
			So(plain.Drain(), ShouldBeEmpty)
		})

		Convey("Both invariants hold after every append of a long repetitive stream", func() {
			tokens := []string{"R00", "R03", "R07", "R11", "R14"}
			state := uint32(1)

			for step := 0; step < 3000; step++ {
				state = state*1664525 + 1013904223
				// A biased stream so long repeats occur.
				grammar.Append(tokens[(state>>24)%uint32(len(tokens)-step%2)])
				So(grammar.Check(), ShouldBeEmpty)
			}

			So(grammar.StartRule().Terminals(), ShouldHaveLength, 3000)
		})

		Convey("Every binary stream up to 14 symbols and ternary up to 9 keeps both invariants and its exact expansion", func() {
			// bbaaabbabaaa broke digram uniqueness before the triple fix in
			// substitute; aaaaaa broke it when the new body was indexed
			// before the substitutions.
			for _, alphabet := range [][]string{{"a", "b"}, {"a", "b", "c"}} {
				limit := map[int]int{2: 14, 3: 9}[len(alphabet)]

				for length := 1; length <= limit; length++ {
					total := 1

					for range length {
						total *= len(alphabet)
					}

					for code := range total {
						stream := NewGrammar()
						sequence := make([]string, 0, length)

						for digit := code; len(sequence) < length; digit /= len(alphabet) {
							sequence = append(sequence, alphabet[digit%len(alphabet)])
							stream.Append(sequence[len(sequence)-1])

							if violations := stream.Check(); len(violations) > 0 {
								So(strings.Join(sequence, "")+": "+strings.Join(violations, "; "), ShouldBeEmpty)
							}
						}

						if expansion := strings.Join(stream.StartRule().TerminalNames(stream), ""); expansion != strings.Join(sequence, "") {
							So(expansion, ShouldEqual, strings.Join(sequence, ""))
						}
					}
				}
			}
		})

		Convey("Check reports a rule referenced once", func() {
			for _, token := range []string{"a", "b", "a", "b"} {
				grammar.Append(token)
			}

			grammar.Rules()[0].count = 1
			So(grammar.Check(), ShouldContain, "rule utility: M1 is referenced 1 time(s)")
		})
	})
}

func TestSequitur(t *testing.T) {
	Convey("Given the live Sequitur sink over stored-shape ticks, gate off", t, func() {
		defer withGate(false, 0)()
		types.SetFocus("BTC/USD")
		defer types.SetFocus("BTC/USD")

		sink := &frameSink{}
		pol := &policy{epoch: 42, actions: map[string]string{}}
		tree, err := newCognitionTree(pol, 2)
		So(err, ShouldBeNil)
		sequitur, err := NewSequitur(context.Background(), sink, tree)
		So(err, ShouldBeNil)

		// Pinned metrics in distinct regions, moving differently each tick so
		// the gate fires changing chords.
		metric := func(tick uint64) map[string]map[string]float64 {
			wave := func(period uint64) float64 { return float64(1 + (tick*7)%period) }

			return map[string]map[string]float64{
				"cvd":         {"midpoint_log_return": wave(3), "flow_aligned_midpoint_return": wave(5)},
				"depthflow":   {"mutation_activity_imbalance": wave(4), "touch_notional:bid": wave(7)},
				"correlation": {"cohort_covariance_score": wave(6), "historical_path_distance": wave(2)},
			}
		}

		ticks := make([]*data.Value, 0)

		for tick := uint64(1); tick <= 200; tick++ {
			ticks = append(ticks, gridTick(tick, metric(tick), "BTC/USD", "ETH/USD"))
		}

		write := func(value *data.Value) {
			_, err := io.Copy(sequitur, value)
			So(err, ShouldBeNil)
		}

		Convey("It streams frames for the focused symbol only, each one consistent", func() {
			for _, value := range ticks {
				write(value)
			}

			So(sink.values, ShouldNotBeEmpty)
			appended := 0
			var last SequiturFrame

			for _, value := range sink.values {
				So(value.Origin, ShouldEqual, data.SequiturOrigin)
				So(value.Role, ShouldEqual, data.SequiturRole)
				So(value.Scope, ShouldEqual, "BTC/USD")

				raw := data.Layer[[]byte](value, 0)
				So(raw, ShouldNotBeNil)
				So(value.Index[0], ShouldEqual, data.LayerGrammar)

				var frame SequiturFrame
				So(json.Unmarshal(*raw, &frame), ShouldBeNil)
				So(frame.Symbol, ShouldEqual, "BTC/USD")
				So(frame.Tick, ShouldEqual, value.Tick)
				So(frame.Fired, ShouldNotBeEmpty)
				So(frame.Path, ShouldNotBeEmpty)
				So(frame.Violations, ShouldBeEmpty)
				So(frame.Policy.Epoch, ShouldEqual, 42)
				So(frame.Gate, ShouldBeNil)
				So(frame.Blocked, ShouldBeFalse)

				for _, region := range frame.Active {
					So(region.Brightness, ShouldBeGreaterThan, 0)
				}

				for _, event := range frame.Events {
					if event.Kind == EventAppend {
						appended++
					}
				}

				last = frame
			}

			// Every terminal in R0's expansion arrived as an append event.
			So(last.RawLength, ShouldEqual, appended)
			So(last.GrammarSymbols, ShouldBeLessThanOrEqualTo, last.RawLength)
		})

		Convey("A stored path the recent tokens end on is matched with its depth and action", func() {
			for _, value := range ticks[:100] {
				write(value)
			}

			var frame SequiturFrame
			So(json.Unmarshal(*data.Layer[[]byte](sink.values[len(sink.values)-1], 0), &frame), ShouldBeNil)

			// The Trainer keys policies by the tape's path; the live tree
			// is asked for the deepest stored suffix of the token stream.
			pol.actions[frame.Path] = "enter"
			sequitur.tree, err = newCognitionTree(pol, 2)
			So(err, ShouldBeNil)
			sink.values = nil

			for _, value := range ticks[100:] {
				write(value)
			}

			stored := 0

			for _, value := range sink.values {
				var next SequiturFrame
				So(json.Unmarshal(*data.Layer[[]byte](value, 0), &next), ShouldBeNil)
				So(next.Policy.Trading, ShouldBeFalse)
				So(next.Policy.Outcome, ShouldBeEmpty)

				if next.Path == frame.Path {
					So(next.Policy.Stored, ShouldBeTrue)
				}

				if next.Policy.Stored {
					So(next.Policy.Match, ShouldEqual, frame.Path)
					So(next.Policy.Depth, ShouldEqual, len(strings.Split(frame.Path, "/")))
					So(next.Policy.Action, ShouldEqual, "enter")
					stored++
				}
			}

			So(stored, ShouldBeGreaterThan, 0)

			// Every symbol's match reaches the cognitive tree tab.
			symbols := map[string]bool{}
			storedMatches := 0

			for _, value := range sink.cognition {
				So(value.Role, ShouldEqual, data.CognitionMatchRole)
				So(value.Index[0], ShouldEqual, data.LayerCognition)

				var match CognitionMatchFrame
				So(json.Unmarshal(*data.Layer[[]byte](value, 0), &match), ShouldBeNil)
				So(match.Symbol, ShouldEqual, value.Scope)
				So(match.Tokens, ShouldNotBeEmpty)
				symbols[match.Symbol] = true

				if match.Stored {
					So(match.Match, ShouldEqual, frame.Path)
					So(match.Action, ShouldEqual, "enter")
					storedMatches++
				}
			}

			So(symbols, ShouldContainKey, "ETH/USD")
			So(storedMatches, ShouldBeGreaterThan, 0)
		})

		Convey("With paper trading on, every matched action reaches Training.Act", func() {
			for _, value := range ticks[:100] {
				write(value)
			}

			var frame SequiturFrame
			So(json.Unmarshal(*data.Layer[[]byte](sink.values[len(sink.values)-1], 0), &frame), ShouldBeNil)

			pol.actions[frame.Path] = "enter"
			pol.keys = []string{frame.Path + "/enter.bin"}
			pol.metrics = map[string]map[string]float64{frame.Path + "/enter.bin": {"gain": 0.01}}
			sequitur.tree, err = newCognitionTree(pol, 2)
			So(err, ShouldBeNil)

			paper := newPaperTraining(sequitur.tree)
			sequitur.Trade(paper.training)
			sink.values = nil

			for _, value := range ticks[100:] {
				write(value)
			}

			outcomes := 0

			for _, value := range sink.values {
				var next SequiturFrame
				So(json.Unmarshal(*data.Layer[[]byte](value, 0), &next), ShouldBeNil)
				So(next.Policy.Trading, ShouldBeTrue)
				So(next.Policy.Outcome == "", ShouldEqual, !next.Policy.Stored)

				if next.Policy.Stored {
					outcomes++
				}
			}

			So(outcomes, ShouldBeGreaterThan, 0)
			So(<-paper.filling.writes, ShouldEqual, "buy")
		})

		Convey("The frame's JSON fields are the ones frontend/src/collections/sequitur.ts reads", func() {
			buf, err := json.Marshal(SequiturFrame{})
			So(err, ShouldBeNil)

			fields := map[string]any{}
			So(json.Unmarshal(buf, &fields), ShouldBeNil)
			So(slices.Sorted(maps.Keys(fields)), ShouldResemble, []string{
				"active", "blocked", "completed", "epoch", "events", "fired", "gate", "grammarSymbols", "held", "midpoint",
				"path", "policy", "rawLength", "rules", "start", "symbol", "tick", "timestamp", "violations",
			})
			So(fields["epoch"], ShouldEqual, "0")

			buf, err = json.Marshal(SequiturPolicy{})
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, `{"epoch":"0","paths":0,"trading":false,"stored":false,"match":"","depth":0,"action":"","outcome":""}`)

			z := 1.5
			buf, err = json.Marshal(GateReading{Surprise: 0.25, Z: &z, Warm: true, Passed: true})
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, `{"surprise":0.25,"z":1.5,"warm":true,"passed":true}`)

			buf, err = json.Marshal(GrammarEvent{})
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, `{"kind":"","rule":0,"into":0,"from":0,"digram":["",""]}`)
		})

		Convey("A tick that is not a stored tick is a loud error", func() {
			_, err := sequitur.Write([]byte("not a value"))
			So(err, ShouldNotBeNil)
		})
	})
}
