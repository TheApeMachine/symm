package hindsight

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/store/storetest"
)

const (
	run    = uint64(1792187257188073000)
	older  = uint64(1792100000000000000)
	btcUSD = "BTC/USD"
	ethUSD = "ETH/USD"
)

/*
storedTick is a tick as collect writes it: the trade frame and each traded
symbol's price layer. A nil ask leaves that side out of the price layer.
*/
func storedTick(epoch, tick uint64, symbols map[string][2]*decimal.Decimal, order ...string) *data.Value {
	trades := make([]kraken.TradeData, 0, len(order))

	for idx, symbol := range order {
		trades = append(trades, kraken.TradeData{
			Symbol: symbol, Side: "buy", Price: *decimal.NewFromFloat64(100 + float64(tick)), Qty: 0.25,
			OrderType: "market", TradeID: int64(tick*10) + int64(idx), Timestamp: time.Unix(int64(tick), 0).UTC(),
		})
	}

	frame, err := sonic.Marshal(map[string]any{"channel": "trade", "type": "update", "data": trades})
	So(err, ShouldBeNil)

	value := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
	value.Epoch, value.Tick, value.Timestamp = epoch, tick, tick*uint64(time.Second)
	value.FormatPrefix()
	value.AppendJSON(frame, data.TradeIndex("update"))

	for _, symbol := range order {
		touch := symbols[symbol]
		price, err := sonic.Marshal(broker.PriceLayer{Symbol: symbol, Bid: touch[0], Ask: touch[1]})
		So(err, ShouldBeNil)
		value.AppendJSON(price, data.SymbolIndex(data.LayerPrice, symbol))
	}

	return value
}

func put(backend *storetest.Backend, value *data.Value) {
	backend.Objects[value.Prefix] = value.Bytes()
}

/*
frames decodes a stream: serialized Values back to back.
*/
func frames(raw []byte) []*data.Value {
	reader := bufio.NewReader(bytes.NewReader(raw))
	out := make([]*data.Value, 0)

	for {
		value := data.New()
		n, err := value.ReadFrom(reader)

		if n == 0 && errors.Is(err, io.EOF) {
			return out
		}

		So(err, ShouldBeNil)
		out = append(out, value)
	}
}

func roles(values []*data.Value) []string {
	out := make([]string, 0, len(values))

	for _, value := range values {
		out = append(out, value.Role)
	}

	return out
}

/*
writes records every Write separately: one hub message each.
*/
type writes struct{ messages [][]byte }

func (writes *writes) Write(p []byte) (int, error) {
	writes.messages = append(writes.messages, p)
	return len(p), nil
}

func TestService(t *testing.T) {
	Convey("Given a bucket holding a trained run and an untrained one", t, func() {
		ctx := context.Background()
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()
		backend.Buckets["symm"] = true

		bid, ask := decimal.NewFromFloat64(99.5), decimal.NewFromFloat64(100.5)
		both := map[string][2]*decimal.Decimal{btcUSD: {bid, ask}, ethUSD: {bid, nil}}

		put(backend, storedTick(run, 1, both, btcUSD, ethUSD))
		put(backend, storedTick(run, 2, both, btcUSD))
		put(backend, storedTick(run, 3, both, ethUSD))
		put(backend, storedTick(older, 1, both, btcUSD))

		excursion := data.NewValue(data.ExcursionOrigin, "up", btcUSD)
		excursion.Epoch, excursion.Tick = run, 2
		excursion.FormatPrefix()
		moves := make([]float64, len(data.ExcursionKeys))
		moves[slices.Index(data.ExcursionKeys, "b_price")] = 100
		moves[slices.Index(data.ExcursionKeys, "c_price")] = 103
		data.AppendVector(excursion, moves, data.ExcursionKeys...)
		put(backend, excursion)

		// Train mints its own policy epoch; the gate tag names the ticks.
		policyEpoch := uint64(1792190000000000000)
		trained := func(epoch uint64, gated bool) {
			policy := data.NewValue(data.PolicyOrigin, "enter", data.PolicyScope)
			policy.Epoch, policy.Tick = epoch, 1
			policy.FormatPrefix()
			So(data.AppendPolicy(policy, "R01/R04", []float64{0.01, 30, 4, 0.01, 0.6}), ShouldBeNil)
			So(data.AppendPolicyGate(policy, data.PolicyGate{TickEpoch: run, Enabled: gated, SurpriseZ: 1, Warmup: data.PolicyWarmup{Rule: data.WarmupSettled, FastRate: 0.15, SlowRate: 0.01}, LearningRate: 0.01}), ShouldBeNil)
			put(backend, policy)
		}

		trained(policyEpoch-1, false)
		trained(policyEpoch, true)

		paper := func(policyEpoch, seq uint64) {
			record := data.NewValue(data.PaperOrigin, "exit", btcUSD)
			record.Epoch, record.Tick = 1792200000000000000, seq
			record.FormatPrefix()
			So(data.AppendPaper(record, "R01/R04", policyEpoch, make([]float64, len(data.PaperKeys))), ShouldBeNil)
			put(backend, record)
		}

		paper(policyEpoch, 1)
		paper(policyEpoch-1, 2)

		reader, err := store.NewReader(ctx, "symm", storetest.Options(server.URL))
		So(err, ShouldBeNil)

		service, err := NewService(ctx, reader)
		So(err, ShouldBeNil)

		stored := len(backend.Objects)

		Convey("It needs a reader", func() {
			_, err := NewService(ctx, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("GET /hindsight/runs lists runs newest first, with decimal string ids", func() {
			app := fiber.New()
			service.Register(app)

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/hindsight/runs", nil))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)

			body, err := io.ReadAll(resp.Body)
			So(err, ShouldBeNil)

			var runs []Run
			So(sonic.Unmarshal(body, &runs), ShouldBeNil)
			So(runs, ShouldHaveLength, 2)
			So(runs[0].ID, ShouldEqual, strconv.FormatUint(run, 10))
			So(runs[0].Trained, ShouldBeTrue)
			So(runs[0].Policies, ShouldResemble, []PolicyInfo{
				{Epoch: strconv.FormatUint(policyEpoch, 10), Gate: true, SurpriseZ: 1, Warmup: "settled: error average at 0.15 no longer below its average at 0.01", LearningRate: 0.01},
				{Epoch: strconv.FormatUint(policyEpoch-1, 10), Gate: false, SurpriseZ: 1, Warmup: "settled: error average at 0.15 no longer below its average at 0.01", LearningRate: 0.01},
			})
			So(runs[1].Policies, ShouldBeEmpty)
			So(runs[1].ID, ShouldEqual, strconv.FormatUint(older, 10))
			So(runs[1].Trained, ShouldBeFalse)
			So(string(body), ShouldContainSubstring, `"id":"1792187257188073000"`)
		})

		Convey("A symbol's stream: the newest policy epoch trained on it, excursions, paper records, ticks, then done", func() {
			var out bytes.Buffer
			So(service.Stream(ctx, &out, run, btcUSD), ShouldBeNil)

			values := frames(out.Bytes())
			So(roles(values), ShouldResemble, []string{
				data.PolicyOrigin, data.ExcursionOrigin, data.PaperOrigin,
				data.HindsightTick, data.HindsightTick, data.HindsightDone,
			})

			for _, value := range values {
				So(value.Origin, ShouldEqual, data.HindsightOrigin)
				So(value.Scope, ShouldEqual, btcUSD)
				So(value.Epoch, ShouldEqual, run)
			}

			// Forwarded Values keep their stored key in their index.
			So(values[1].Index[0], ShouldStartWith, excursion.Prefix)
			So(values[2].Index[0], ShouldContainSubstring, "policy_epoch:"+strconv.FormatUint(policyEpoch, 10))

			tick := data.Metrics(values[4])
			So(values[4].Tick, ShouldEqual, 2)
			So(values[4].Timestamp, ShouldEqual, 2*uint64(time.Second))
			So(tick["bid"], ShouldEqual, 99.5)
			So(tick["ask"], ShouldEqual, 100.5)
			So(tick["trade_price"], ShouldEqual, 102)
			So(tick["trade_qty"], ShouldEqual, 0.25)
			So(tick["trades"], ShouldEqual, 1)

			So(data.Metrics(values[5]), ShouldResemble, map[string]float64{
				"ticks": 2, "symbols": 1, "excursions": 1, "policies": 1, "papers": 1,
			})

			// Reading never wrote.
			So(backend.Objects, ShouldHaveLength, stored)
		})

		Convey("A side the price layer does not hold is NaN, not zero", func() {
			var out bytes.Buffer
			So(service.Stream(ctx, &out, run, ethUSD), ShouldBeNil)

			values := frames(out.Bytes())
			tick := data.Metrics(values[1])
			So(values[1].Role, ShouldEqual, data.HindsightTick)
			So(tick["bid"], ShouldEqual, 99.5)
			So(math.IsNaN(tick["ask"]), ShouldBeTrue)
		})

		Convey("With no symbol it summarizes every symbol the run traded", func() {
			var out bytes.Buffer
			So(service.Stream(ctx, &out, run, ""), ShouldBeNil)

			values := frames(out.Bytes())
			So(roles(values), ShouldResemble, []string{data.HindsightSymbol, data.HindsightSymbol, data.HindsightDone})
			So(values[0].Scope, ShouldEqual, "")
			So(values[0].Index[0], ShouldStartWith, data.Prefix(data.HindsightOrigin, data.HindsightSymbol, btcUSD, run))
			So(values[1].Index[0], ShouldStartWith, data.Prefix(data.HindsightOrigin, data.HindsightSymbol, ethUSD, run))

			btc := data.Metrics(values[0])
			So(btc["ticks"], ShouldEqual, 2)
			So(btc["first_tick"], ShouldEqual, 1)
			So(btc["last_tick"], ShouldEqual, 2)
			So(btc["low"], ShouldEqual, 101)
			So(btc["high"], ShouldEqual, 102)
			So(btc["up"], ShouldEqual, 1)
			So(btc["touched"], ShouldEqual, 2)
			So(btc["top_excursion"], ShouldAlmostEqual, 0.03, 1e-12)
			So(data.Metrics(values[1])["touched"], ShouldEqual, 0)
			So(data.Metrics(values[2])["symbols"], ShouldEqual, 2)
		})

		Convey("A symbol the run never traded ends on an error frame", func() {
			var out bytes.Buffer
			err := service.Stream(ctx, &out, run, "DOGE/USD")
			So(err, ShouldNotBeNil)

			values := frames(out.Bytes())
			last := values[len(values)-1]
			So(last.Role, ShouldEqual, data.HindsightError)
			So(last.Index[len(last.Index)-1], ShouldEqual, data.LayerHindsightError)
			So(string(last.Layers[len(last.Layers)-1]), ShouldContainSubstring, "DOGE/USD")
		})

		Convey("A stored tick that does not decode ends the stream on an error frame naming it", func() {
			bad := data.TickPrefix(run) + "0000000000000000004.bin"
			backend.Objects[bad] = []byte("garbage")

			var out bytes.Buffer
			So(service.Stream(ctx, &out, run, btcUSD), ShouldNotBeNil)

			values := frames(out.Bytes())
			last := values[len(values)-1]
			So(last.Role, ShouldEqual, data.HindsightError)
			So(string(last.Layers[len(last.Layers)-1]), ShouldContainSubstring, bad)
		})

		Convey("A canceled request returns its cause and writes no error frame", func() {
			canceled, cancel := context.WithCancel(ctx)
			cancel()

			var out bytes.Buffer
			So(errors.Is(service.Stream(canceled, &out, run, btcUSD), context.Canceled), ShouldBeTrue)

			for _, value := range frames(out.Bytes()) {
				So(value.Role, ShouldNotEqual, data.HindsightError)
			}
		})

		Convey("Every message holds whole Values", func() {
			for tick := uint64(10); tick < 2000; tick++ {
				put(backend, storedTick(run, tick, both, btcUSD))
			}

			sink := &writes{}
			So(service.Stream(ctx, sink, run, btcUSD), ShouldBeNil)
			So(len(sink.messages), ShouldBeGreaterThan, 1)

			total := 0

			for _, message := range sink.messages {
				So(len(message), ShouldBeLessThan, 2*batchBytes)
				total += len(frames(message))
			}

			So(total, ShouldEqual, 3+2+1990+1)
		})
	})
}
