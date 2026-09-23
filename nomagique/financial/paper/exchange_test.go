package paper_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/financial/paper"
	"github.com/theapemachine/symm/nomagique/transport"
)

const schedule = `{"XBTUSD":{"altname":"XBTUSD","wsname":"BTC/USD","quote":"USD","cost_decimals":5,` +
	`"fees":[[0,0.40],[10000,0.35]]},"ETHEUR":{"altname":"ETHEUR","wsname":"ETH/EUR","quote":"EUR","cost_decimals":5,"fees":[[0,0.40]]}}`

type resting struct{ price, quantity, at string }

var (
	bids = []resting{{"99", "1", "2026-09-23T09:00:00Z"}, {"98", "3", "2026-09-23T09:00:01Z"}}
	asks = []resting{{"100", "1", "2026-09-23T09:00:02Z"}, {"101", "2", "2026-09-23T09:00:03Z"}}
)

/*
level3Frame renders a Kraken level3 frame. The checksum is the one the
exchange would send for the book after applying it, computed by the SDK's own
book, so the venue's reconciliation path is the one under test.
*/
func level3Frame(kind, symbol string, before, after [2][]resting, event string) []byte {
	replica := book.New()
	records := func(direction book.BookDirection, orders []resting, emit bool) []map[string]any {
		out := make([]map[string]any, 0, len(orders))

		for _, placed := range orders {
			price, err := decimal.NewFromString(placed.price)
			So(err, ShouldBeNil)
			quantity, err := decimal.NewFromString(placed.quantity)
			So(err, ShouldBeNil)
			at, err := time.Parse(time.RFC3339, placed.at)
			So(err, ShouldBeNil)
			identity := placed.at + placed.price

			if emit && event == "delete" {
				quantity = decimal.NewFromInt64(0)
			}
			replica.Update(&book.UpdateOptions{Direction: direction, ID: identity, Price: price, Quantity: quantity, Timestamp: at})

			if !emit {
				continue
			}
			record := map[string]any{
				"order_id": identity, "limit_price": json.Number(placed.price),
				"order_qty": json.Number(placed.quantity), "timestamp": placed.at,
			}

			if event != "" {
				record["event"] = event
			}
			out = append(out, record)
		}
		return out
	}
	records(book.Bid, before[0], false)
	records(book.Ask, before[1], false)
	entry := map[string]any{
		"symbol": symbol, "bids": records(book.Bid, after[0], true), "asks": records(book.Ask, after[1], true),
	}
	entry["checksum"] = json.Number(replica.L3Checksum("").LocalChecksum)
	frame, err := json.Marshal(map[string]any{"channel": "level3", "type": kind, "data": []any{entry}})
	So(err, ShouldBeNil)
	return frame
}

func snapshot(symbol string) []byte {
	return level3Frame("snapshot", symbol, [2][]resting{}, [2][]resting{bids, asks}, "")
}

/* deep is a bid far from the touch: adding or removing it moves the book but not its fills. */
var deep = resting{"90", "1", "2026-09-23T09:30:00Z"}

func deepBid(event string) []byte {
	before := [2][]resting{bids, asks}

	if event == "delete" {
		before[0] = append(append([]resting{}, bids...), deep)
	}
	return level3Frame("update", "BTC/USD", before, [2][]resting{{deep}, nil}, event)
}

func instrumentFrame(symbol, minimumQuantity string) []byte {
	frame, err := json.Marshal(map[string]any{
		"channel": "instrument", "type": "snapshot",
		"data": map[string]any{"pairs": []any{map[string]any{
			"symbol": symbol, "qty_min": json.Number(minimumQuantity),
			"cost_min": json.Number("0.5"), "qty_increment": json.Number("0.0001"),
		}}},
	})
	So(err, ShouldBeNil)
	return frame
}

func at(second int) string {
	return "2026-09-23T10:00:" + strconv.Itoa(10+second) + "Z"
}

func configure(params paper.Exchange_write_Params, moment string) error {
	params.SetDepth(10)

	for _, err := range []error{
		params.SetTime(moment),
		params.SetCapital("1000"),
		params.SetCurrency("USD"),
		params.SetFraction("0.2"),
		params.SetSchedule([]byte(schedule)),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

/* observe is one evaluation: the frame is written and the account reported. */
func observe(client paper.Exchange, frame []byte, moment string) paper.Account {
	ctx := context.Background()
	So(client.Write(ctx, func(params paper.Exchange_write_Params) error {
		if err := configure(params, moment); err != nil {
			return err
		}
		return params.SetFrame(frame)
	}), ShouldBeNil)
	So(client.WaitStreaming(), ShouldBeNil)
	future, release := client.Done(ctx, nil)
	Reset(release)
	result, err := future.Struct()
	So(err, ShouldBeNil)
	return result
}

/* commit is the program writing a decision back after the evaluation, without done. */
func commit(client paper.Exchange, symbol, action, moment string) error {
	ctx := context.Background()
	err := client.Write(ctx, func(params paper.Exchange_write_Params) error {
		if err := configure(params, moment); err != nil {
			return err
		}
		return first(params.SetSymbol(symbol), params.SetAction(action))
	})

	if err != nil {
		return err
	}
	return client.WaitStreaming()
}

/* primed is an exchange that knows BTC/USD's rules and holds its reconciled book. */
func primed(minimumQuantity string) paper.Exchange {
	client := paper.Exchange_ServerToClient(paper.NewExchange())
	observe(client, instrumentFrame("BTC/USD", minimumQuantity), at(0))
	account := observe(client, snapshot("BTC/USD"), at(1))
	So(account.Books(), ShouldEqual, 1)
	return client
}

func first(results ...error) error {
	for _, err := range results {
		if err != nil {
			return err
		}
	}
	return nil
}

func read(value string, err error) string {
	So(err, ShouldBeNil)
	return value
}

func number(text string) *decimal.Decimal {
	value, err := decimal.NewFromString(text)
	So(err, ShouldBeNil)
	return value
}

func held(account paper.Account) []string {
	list, err := account.Holding()
	So(err, ShouldBeNil)
	symbols := make([]string, list.Len())

	for index := range symbols {
		symbols[index] = read(list.At(index))
	}
	return symbols
}

var exchangeGraph = `{"nodes":{
"input":{"id":"input","type":"transport.Fan","connections":{"outputs":{"out":[
  {"nodeId":"frame","portName":"data"},{"nodeId":"time","portName":"data"},{"nodeId":"policy","portName":"data"}]}}},
"frame":{"id":"frame","type":"data.Extract","inputData":{"path":{"value":"frame"},"encoding":{"value":"json"}},
  "connections":{"inputs":{"data":[{"nodeId":"input","portName":"out"}]},"outputs":{"json":[{"nodeId":"exchange","portName":"frame"}]}}},
"time":{"id":"time","type":"data.Extract","inputData":{"path":{"value":"time"},"encoding":{"value":"text"}},
  "connections":{"inputs":{"data":[{"nodeId":"input","portName":"out"}]},"outputs":{"text":[{"nodeId":"exchange","portName":"time"}]}}},
"exchange":{"id":"exchange","type":"paper.Exchange","inputData":{"depth":{"value":10},"capital":{"value":"1000"},
  "currency":{"value":"USD"},"fraction":{"value":"0.2"},"schedule":{"value":` + strconv.Quote(schedule) + `}},
  "connections":{"inputs":{"frame":[{"nodeId":"frame","portName":"json"}],"time":[{"nodeId":"time","portName":"text"}],
    "symbol":[{"nodeId":"symbol","portName":"text"}],"action":[{"nodeId":"action","portName":"text"}]},
  "outputs":{"resting":[{"nodeId":"policy","portName":"unsigned"}]}}},
"policy":{"id":"policy","type":"data.Insert","inputData":{"path":{"value":"resting"},"encoding":{"value":"uint64"}},
  "connections":{"inputs":{"data":[{"nodeId":"input","portName":"out"}],"unsigned":[{"nodeId":"exchange","portName":"resting"}]},
  "outputs":{"out":[{"nodeId":"symbol","portName":"data"},{"nodeId":"action","portName":"data"}]}}},
"symbol":{"id":"symbol","type":"data.Extract","inputData":{"path":{"value":"decision.symbol"},"encoding":{"value":"text"}},
  "connections":{"inputs":{"data":[{"nodeId":"policy","portName":"out"}]},"outputs":{"text":[{"nodeId":"exchange","portName":"symbol"}]}}},
"action":{"id":"action","type":"data.Extract","inputData":{"path":{"value":"decision.action"},"encoding":{"value":"text"}},
  "connections":{"inputs":{"data":[{"nodeId":"policy","portName":"out"}]},"outputs":{"text":[{"nodeId":"exchange","portName":"action"}]}}}
}}`

func TestExchangeProgram(t *testing.T) {
	Convey("Given a compiled graph whose decision is wired back into the exchange", t, func() {
		program, err := compiler.CompileJSON([]byte(exchangeGraph), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()

		step := func(frame []byte, moment string, decision map[string]string) paper.Account {
			document := map[string]any{"frame": json.RawMessage(frame), "time": moment}

			if decision != nil {
				document["decision"] = decision
			}
			payload, err := json.Marshal(document)
			So(err, ShouldBeNil)
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			params, err := transport.NewFan_write_Params(segment)
			So(err, ShouldBeNil)
			So(params.SetData(payload), ShouldBeNil)
			So(program.Execute(context.Background(), map[compiler.NodeID]capnp.Struct{
				program.NodeMap["input"]: capnp.Struct(params),
			}), ShouldBeNil)
			result, found := program.Result("exchange")
			So(found, ShouldBeTrue)
			return paper.Account(result)
		}

		Convey("When it enters on the snapshot and exits two frames later", func() {
			step(instrumentFrame("BTC/USD", "0.001"), at(0), nil)
			entered := step(snapshot("BTC/USD"), at(1), map[string]string{"symbol": "BTC/USD", "action": "ENTER"})
			filled := step(deepBid("add"), at(2), nil)
			exited := step(deepBid("delete"), at(3), map[string]string{"symbol": "BTC/USD", "action": "EXIT"})
			closed := step(deepBid("add"), at(4), nil)

			Convey("Then nothing is spent on the frame the entry was decided on", func() {
				So(held(entered), ShouldBeEmpty)
				So(number(read(entered.Cash())).Cmp(number("1000")), ShouldEqual, 0)
			})

			Convey("Then the next book frame fills it through the asks, fee included", func() {
				// 200 / 1.004 = 199.20318; 1 @ 100 then 0.9822 @ 101 = 199.2022; fee 0.7968088.
				So(held(filled), ShouldResemble, []string{"BTC/USD"})
				So(number(read(filled.Cash())).Cmp(number("800.0009912")), ShouldEqual, 0)
				So(held(exited), ShouldResemble, []string{"BTC/USD"})
			})

			Convey("Then the exit sells through the bids and reports what the round trip made", func() {
				// 1 @ 99 + 0.9822 @ 98 = 195.2556; fee 0.7810224; net 194.4745776.
				So(closed.Which(), ShouldEqual, paper.Account_Which_closed)
				trip := closed.Closed()
				So(read(trip.Symbol()), ShouldEqual, "BTC/USD")
				So(number(read(trip.Basis())).Cmp(number("199.9990088")), ShouldEqual, 0)
				So(number(read(trip.Proceeds())).Cmp(number("194.4745776")), ShouldEqual, 0)
				So(number(read(trip.Pnl())).Cmp(number("-5.5244312")), ShouldEqual, 0)
				So(read(trip.Opened()), ShouldEqual, at(2))
				So(read(trip.ClosedAt()), ShouldEqual, at(4))
				So(number(read(closed.Cash())).Cmp(number("994.4755688")), ShouldEqual, 0)
				So(held(closed), ShouldBeEmpty)
			})
		})
	})
}
