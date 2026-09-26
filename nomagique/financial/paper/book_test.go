package paper_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/paper"
	marketfixture "github.com/theapemachine/symm/tests/market"
)

type resting = marketfixture.Order

var (
	bids = []resting{{"99", "1", "2026-09-23T09:00:00Z"}, {"98", "3", "2026-09-23T09:00:01Z"}}
	asks = []resting{{"100", "1", "2026-09-23T09:00:02Z"}, {"101", "2", "2026-09-23T09:00:03Z"}}
	deep = resting{"90", "1", "2026-09-23T09:30:00Z"}
)

func snapshotFrame(symbol string) []byte {
	return marketfixture.Level3Frame("snapshot", symbol, [2][]resting{}, [2][]resting{bids, asks}, "")
}

/* deepBidFrame adds or removes a bid far from the touch: the book moves, its fills do not. */
func deepBidFrame(event string) []byte {
	before := [2][]resting{bids, asks}

	if event == "delete" {
		before[0] = append(append([]resting{}, bids...), deep)
	}
	return marketfixture.Level3Frame("update", "BTC/USD", before, [2][]resting{{deep}, nil}, event)
}

func instrumentFrame(symbol, minimumQuantity string) []byte {
	frame, err := json.Marshal(map[string]any{
		"channel": "instrument", "type": "snapshot",
		"data": map[string]any{"pairs": []any{map[string]any{
			"symbol": symbol, "quote": "USD", "cost_precision": json.Number("5"),
			"qty_min": json.Number(minimumQuantity), "cost_min": json.Number("0.5"), "qty_increment": json.Number("1e-04"),
		}}},
	})
	So(err, ShouldBeNil)
	return frame
}

type market struct {
	Symbol  string          `json:"symbol"`
	Bids    [][2]string     `json:"bids"`
	Asks    [][2]string     `json:"asks"`
	Pair    json.RawMessage `json:"pair"`
	Values  []float64       `json:"-"`
	Present []bool          `json:"-"`
}

func replay(client paper.Book, frame []byte) (market, error) {
	ctx := context.Background()

	if err := client.Write(ctx, func(params paper.Book_write_Params) error {
		params.SetDepth(10)
		frames, err := params.NewFrame(1)

		if err != nil {
			return err
		}
		return frames.Set(0, frame)
	}); err != nil {
		return market{}, err
	}

	if err := client.WaitStreaming(); err != nil {
		return market{}, err
	}
	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		return market{}, err
	}
	out, err := results.Out()

	if err != nil {
		return market{}, err
	}
	var reported market
	err = json.Unmarshal(bytes.Clone(out), &reported)
	if err != nil {
		return reported, err
	}
	values, err := results.Values()
	if err != nil {
		return reported, err
	}
	present, err := results.Present()
	if err != nil {
		return reported, err
	}
	for index := range values.Len() {
		reported.Values = append(reported.Values, values.At(index))
		reported.Present = append(reported.Present, present.At(index))
	}
	return reported, err
}

/* BenchmarkBookWrite exercises reconciliation and native projection at the RPC boundary. */
func BenchmarkBookWrite(b *testing.B) {
	ctx := context.Background()
	client := paper.Book_ServerToClient(paper.NewBook(ctx))
	defer client.Release()
	frames := [][]byte{snapshotFrame("BTC/USD"), deepBidFrame("add"), deepBidFrame("delete")}
	b.ReportAllocs()
	iteration := 0
	for b.Loop() {
		frame := frames[iteration%len(frames)]
		iteration++
		if err := client.Write(ctx, func(params paper.Book_write_Params) error {
			params.SetDepth(10)
			params.SetEncode(false)
			arrivals, err := params.NewFrame(1)
			if err != nil {
				return err
			}
			return arrivals.Set(0, frame)
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		result, err := future.Struct()
		if err != nil {
			release()
			b.Fatal(err)
		}
		present, err := result.Present()
		if err != nil || !present.At(0) {
			release()
			b.Fatalf("native reconciled bid missing: %v", err)
		}
		release()
	}
}

func TestBookWrite(t *testing.T) {
	Convey("Given a book that has seen BTC/USD's instrument record and snapshot", t, func() {
		client := paper.Book_ServerToClient(paper.NewBook(context.Background()))
		defer client.Release()
		quiet, err := replay(client, instrumentFrame("BTC/USD", "0.001"))
		So(err, ShouldBeNil)
		So(quiet.Symbol, ShouldEqual, "")
		reported, err := replay(client, snapshotFrame("BTC/USD"))
		So(err, ShouldBeNil)

		Convey("Then it reports the reconciled levels best first, with the pair's record", func() {
			So(reported.Symbol, ShouldEqual, "BTC/USD")
			So(reported.Bids, ShouldResemble, [][2]string{{"99", "1"}, {"98", "3"}})
			So(reported.Asks, ShouldResemble, [][2]string{{"100", "1"}, {"101", "2"}})
			So(string(reported.Pair), ShouldContainSubstring, `"qty_min":0.001`)
		})

		Convey("When a replayed record carries its entry as one object", func() {
			var frame map[string]any
			So(json.Unmarshal(deepBidFrame("add"), &frame), ShouldBeNil)
			frame["data"] = frame["data"].([]any)[0]
			record, err := json.Marshal(frame)
			So(err, ShouldBeNil)
			reported, err = replay(client, record)
			So(err, ShouldBeNil)
			So(reported.Bids, ShouldResemble, [][2]string{{"99", "1"}, {"98", "3"}, {"90", "1"}})
		})

		Convey("When no frame arrives, it reports no market", func() {
			reported, err = replay(client, nil)
			So(err, ShouldBeNil)
			So(reported.Symbol, ShouldEqual, "")
		})

		Convey("When an update reconciles, the moved book is reported", func() {
			reported, err = replay(client, deepBidFrame("add"))
			So(err, ShouldBeNil)
			So(reported.Bids, ShouldResemble, [][2]string{{"99", "1"}, {"98", "3"}, {"90", "1"}})
		})

		Convey("When the book stops reconciling with the exchange's checksum", func() {
			reported, err = replay(client, []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","checksum":1,"bids":[],"asks":[]}]}`))
			So(err, ShouldBeNil)

			Convey("Then it reports nothing until the next snapshot rather than a guessed book", func() {
				So(reported.Symbol, ShouldEqual, "")
				reported, err = replay(client, deepBidFrame("add"))
				So(err, ShouldBeNil)
				So(reported.Symbol, ShouldEqual, "")
				reported, err = replay(client, snapshotFrame("BTC/USD"))
				So(err, ShouldBeNil)
				So(reported.Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})

	Convey("Given a level3 frame without the subscribed depth", t, func() {
		client := paper.Book_ServerToClient(paper.NewBook(context.Background()))
		defer client.Release()
		So(client.Write(context.Background(), func(params paper.Book_write_Params) error {
			frames, err := params.NewFrame(1)

			if err != nil {
				return err
			}
			return frames.Set(0, snapshotFrame("BTC/USD"))
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldNotBeNil)
	})
}
