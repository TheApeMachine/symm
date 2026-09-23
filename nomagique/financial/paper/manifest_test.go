package paper_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* account is the settled state manifest/paper_exchange.json hands its decision maker. */
type account struct {
	Cash    json.Number `json:"cash"`
	Holding struct {
		Quantity json.Number `json:"quantity"`
		Basis    json.Number `json:"basis"`
		Opened   string      `json:"opened"`
	} `json:"holding"`
	Order struct {
		Side     string      `json:"side"`
		Amount   json.Number `json:"amount"`
		Reserved json.Number `json:"reserved"`
	} `json:"order"`
	Closed *struct {
		Symbol   string      `json:"symbol"`
		Opened   string      `json:"opened"`
		Closed   string      `json:"closed"`
		Basis    json.Number `json:"basis"`
		Proceeds json.Number `json:"proceeds"`
		Pnl      json.Number `json:"pnl"`
	} `json:"closed"`
	Refused *struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	} `json:"refused"`
}

func at(second int) string {
	return "2026-09-23T10:00:" + strconv.Itoa(10+second) + "Z"
}

func fan(program *compiler.Program, node string, document map[string]any) (compiler.NodeID, capnp.Struct) {
	payload, err := json.Marshal(document)
	So(err, ShouldBeNil)
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	So(err, ShouldBeNil)
	params, err := transport.NewFan_write_Params(segment)
	So(err, ShouldBeNil)
	So(params.SetData(payload), ShouldBeNil)
	return program.NodeMap[node], capnp.Struct(params)
}

func TestPaperExchangeManifest(t *testing.T) {
	Convey("Given the paper exchange graph", t, func() {
		program, err := compiler.CompileFile("../../../manifest/paper_exchange.json", nil, compiler.NewRepository())
		So(err, ShouldBeNil)
		defer program.Release()

		step := func(frame []byte, symbol, moment, action string) account {
			event := map[string]any{"frame": json.RawMessage(frame), "time": moment}

			if symbol != "" {
				event["symbol"] = symbol
			}

			if action != "" {
				event["action"] = action
			}
			node, args := fan(program, "event", event)
			So(program.Execute(context.Background(), map[compiler.NodeID]capnp.Struct{node: args}), ShouldBeNil)
			result, found := program.Result("state")

			if !found {
				return account{}
			}
			payload, err := transport.Fan_done_Results(result).Out()
			So(err, ShouldBeNil)
			var settled account
			So(json.Unmarshal(payload, &settled), ShouldBeNil)
			return settled
		}

		Convey("When it enters on BTC/USD and exits two frames later", func() {
			step(instrumentFrame("BTC/USD", "0.001"), "", at(0), "")
			entered := step(snapshotFrame("BTC/USD"), "BTC/USD", at(1), "ENTER")
			filled := step(deepBidFrame("add"), "BTC/USD", at(2), "")
			exiting := step(deepBidFrame("delete"), "BTC/USD", at(3), "EXIT")
			closed := step(deepBidFrame("add"), "BTC/USD", at(4), "")

			Convey("Then nothing is spent on the frame the entry was decided on", func() {
				So(entered.Cash.String(), ShouldEqual, "1000")
				So(entered.Holding.Quantity.String(), ShouldEqual, "")
			})

			Convey("Then the next frame fills a fifth of the cash through the asks, fee included", func() {
				// 200 / 1.008 = 198.41269: 1 @ 100 and 0.9743 @ 101; fee 0.80%.
				So(filled.Holding.Quantity.String(), ShouldEqual, "1.9743")
				So(filled.Holding.Basis.String(), ShouldEqual, "199.9915344")
				So(filled.Holding.Opened, ShouldEqual, at(2))
				So(filled.Cash.String(), ShouldEqual, "800.0084656")
				So(filled.Order.Side, ShouldEqual, "")
				So(exiting.Holding.Quantity.String(), ShouldEqual, "1.9743")
			})

			Convey("Then the exit sells through the bids and reports what the round trip made", func() {
				// 1 @ 99 and 0.9743 @ 98 = 194.4814; fee 1.5558512.
				So(closed.Closed, ShouldNotBeNil)
				So(closed.Closed.Symbol, ShouldEqual, "BTC/USD")
				So(closed.Closed.Basis.String(), ShouldEqual, "199.9915344")
				So(closed.Closed.Proceeds.String(), ShouldEqual, "192.9255488")
				So(closed.Closed.Pnl.String(), ShouldEqual, "-7.0659856")
				So(closed.Closed.Opened, ShouldEqual, at(2))
				So(closed.Closed.Closed, ShouldEqual, at(4))
				So(closed.Cash.String(), ShouldEqual, "992.9340144")
				So(closed.Holding.Quantity.String(), ShouldEqual, "")
			})
		})

		Convey("When the pair's minimum is more than a fifth of the cash buys", func() {
			step(instrumentFrame("BTC/USD", "2"), "", at(0), "")
			step(snapshotFrame("BTC/USD"), "BTC/USD", at(1), "ENTER")
			refused := step(deepBidFrame("add"), "BTC/USD", at(2), "")

			Convey("Then the exchange refuses it whole and the reservation returns to cash", func() {
				So(refused.Refused, ShouldNotBeNil)
				So(refused.Refused.Action, ShouldEqual, "ENTER")
				So(refused.Refused.Reason, ShouldEqual, "below_minimum")
				So(refused.Cash.String(), ShouldEqual, "1000")
				So(refused.Holding.Quantity.String(), ShouldEqual, "")
				So(refused.Order.Side, ShouldEqual, "")
			})
		})

		Convey("When it decides to EXIT while flat", func() {
			step(instrumentFrame("BTC/USD", "0.001"), "", at(0), "")
			flat := step(snapshotFrame("BTC/USD"), "BTC/USD", at(1), "EXIT")
			after := step(deepBidFrame("add"), "BTC/USD", at(2), "")

			Convey("Then nothing is placed", func() {
				So(flat.Order.Side, ShouldEqual, "")
				So(after.Order.Side, ShouldEqual, "")
				So(after.Cash.String(), ShouldEqual, "1000")
			})
		})
	})
}
