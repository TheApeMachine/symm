package tables_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/tests/tablestest"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/wf"
)

func TestDrain(t *testing.T) {
	Convey("Given a ring buffer and an Iceberg catalog", t, func() {
		catalog := tablestest.New(t)
		ring := wf.NewRingBuffer[*data.Measurement[float64]](64)

		viper.Set("hindsight.capture.flush_interval", 10*time.Millisecond)
		viper.Set("hindsight.capture.commit_interval", 50*time.Millisecond)
		viper.Set("hindsight.capture.commit_rows", 100)

		now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

		// 1. Spot Trade
		tradeMeas := data.NewMeasurement[float64]("websocket:public", map[string]data.Metric[float64]{
			"price":    {Raw: 50000.5},
			"qty":      {Raw: 1.25},
			"trade_id": {Raw: 98765},
		})
		tradeMeas.Label = "BTC/USD"
		tradeMeas.At = now
		tradeMeas.Provenance = map[string]string{
			"channel":  "trade",
			"side":     "buy",
			"ord_type": "limit",
		}
		ring.Put(tradeMeas)

		// 2. Spot Ticker
		tickerMeas := data.NewMeasurement[float64]("websocket:public", map[string]data.Metric[float64]{
			"bid":     {Raw: 50000.0},
			"bid_qty": {Raw: 2.0},
			"ask":     {Raw: 50001.0},
			"ask_qty": {Raw: 3.5},
			"last":    {Raw: 50000.5},
			"volume":  {Raw: 100.0},
			"vwap":    {Raw: 50000.2},
		})
		tickerMeas.Label = "BTC/USD"
		tickerMeas.At = now
		tickerMeas.Provenance = map[string]string{
			"channel": "ticker",
		}
		ring.Put(tickerMeas)

		// 3. Futures Trade
		futuresTradeMeas := data.NewMeasurement[float64]("websocket:futures", map[string]data.Metric[float64]{
			"price":    {Raw: 50100.0},
			"qty":      {Raw: 0.5},
			"trade_id": {Raw: 12345},
		})
		futuresTradeMeas.Label = "PF_BTCUSD"
		futuresTradeMeas.At = now
		futuresTradeMeas.Provenance = map[string]string{
			"channel": "futures.trade",
			"side":    "sell",
			"type":    "fill",
		}
		ring.Put(futuresTradeMeas)

		// 4. Executions
		exactQty, _ := decimal.NewFromString("0.75")
		exactPrice, _ := decimal.NewFromString("49999.0")
		exactCost, _ := decimal.NewFromString("37499.25")
		exactFee, _ := decimal.NewFromString("5.5")
		execMeas := data.NewMeasurement[float64]("websocket:private", map[string]data.Metric[float64]{
			"last_qty":      {Exact: exactQty},
			"last_price":    {Exact: exactPrice},
			"cost":          {Exact: exactCost},
			"cum_qty":       {Exact: exactQty},
			"cum_cost":      {Exact: exactCost},
			"avg_price":     {Exact: exactPrice},
			"fee_usd_equiv": {Exact: exactFee},
			"trade_id":      {Raw: 54321},
			"order_userref": {Raw: 1001},
		})
		execMeas.Label = "BTC/USD"
		execMeas.At = now
		execMeas.Provenance = map[string]string{
			"channel":      "executions",
			"order_id":     "ORD-123",
			"exec_id":      "EXEC-456",
			"exec_type":    "trade",
			"side":         "buy",
			"order_type":   "limit",
			"order_status": "filled",
			"fees":         "5.5 USD",
		}
		ring.Put(execMeas)

		// 5. Signal Measurement
		signalMeas := data.NewMeasurement[float64]("signal:cvd", map[string]data.Metric[float64]{
			"cvd": {Raw: 42.195},
		})
		signalMeas.Label = "BTC/USD"
		signalMeas.At = now
		signalMeas.Maturity = 0.95
		signalMeas.SNR = 3.2
		signalMeas.SNRDefined = true
		ring.Put(signalMeas)

		// 6. Spot Level3 Order
		level3Meas := data.NewMeasurement[float64]("websocket:private", map[string]data.Metric[float64]{
			"limit_price": {Raw: 50005.0},
			"order_qty":   {Raw: 2.5},
			"checksum":    {Raw: 123456789},
		})
		level3Meas.Label = "BTC/USD"
		level3Meas.At = now
		level3Meas.Provenance = map[string]string{
			"channel":  "level3",
			"side":     "bid",
			"event":    "add",
			"order_id": "ORD-L3-1",
		}
		ring.Put(level3Meas)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})

		go func() {
			tables.Drain(ctx, catalog, ring)
			close(done)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		<-done

		Convey("Spot trades are persisted to Iceberg and readable", func() {
			trades, err := catalog.SpotTrade(context.Background(), 1, 0)
			So(err, ShouldBeNil)
			So(len(trades), ShouldEqual, 1)
			So(trades[0].Symbol, ShouldEqual, "BTC/USD")
			So(trades[0].Price, ShouldEqual, 50000.5)
			So(trades[0].Qty, ShouldEqual, 1.25)
			So(trades[0].Side, ShouldEqual, "buy")
			So(trades[0].OrdType, ShouldEqual, "limit")
			So(trades[0].TradeID, ShouldEqual, 98765)
		})

		Convey("Spot tickers are persisted to Iceberg and readable", func() {
			tickers, err := catalog.SpotTicker(context.Background(), 1, 0)
			So(err, ShouldBeNil)
			So(len(tickers), ShouldEqual, 1)
			So(tickers[0].Symbol, ShouldEqual, "BTC/USD")
			So(tickers[0].Bid, ShouldEqual, 50000.0)
			So(tickers[0].Ask, ShouldEqual, 50001.0)
			So(tickers[0].Last, ShouldEqual, 50000.5)
		})

		Convey("Futures trades are persisted to Iceberg and readable", func() {
			fTrades, err := catalog.FuturesTrade(context.Background(), 1, 0)
			So(err, ShouldBeNil)
			So(len(fTrades), ShouldEqual, 1)
			So(fTrades[0].Symbol, ShouldEqual, "PF_BTCUSD")
			So(fTrades[0].Price, ShouldEqual, 50100.0)
			So(fTrades[0].Qty, ShouldEqual, 0.5)
			So(fTrades[0].Side, ShouldEqual, "sell")
		})

		Convey("Executions are persisted to Iceberg and readable", func() {
			execs, err := catalog.Executions(context.Background(), 1, 0)
			So(err, ShouldBeNil)
			So(len(execs), ShouldEqual, 1)
			So(execs[0].Symbol, ShouldEqual, "BTC/USD")
			So(execs[0].OrderID, ShouldEqual, "ORD-123")
			So(execs[0].Side, ShouldEqual, "buy")
			So(execs[0].LastPrice.Float64(), ShouldEqual, 49999.0)
			So(execs[0].LastQty.Float64(), ShouldEqual, 0.75)
		})

		Convey("Signal measurements are persisted to Iceberg and readable", func() {
			measurements, err := catalog.Measurements(context.Background(), 1, 0)
			So(err, ShouldBeNil)
			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Symbol, ShouldEqual, "BTC/USD")
			So(measurements[0].Source, ShouldEqual, "signal:cvd")
			So(measurements[0].Metrics["cvd"], ShouldEqual, 42.195)
			So(measurements[0].SNR, ShouldEqual, 3.2)
			So(measurements[0].SNRDefined, ShouldBeTrue)
		})

		Convey("Spot Level3 orders are persisted to Iceberg and readable", func() {
			l3Rows, err := catalog.SpotLevel3(context.Background(), 1, 0)
			So(err, ShouldBeNil)
			So(len(l3Rows), ShouldEqual, 1)
			So(l3Rows[0].Symbol, ShouldEqual, "BTC/USD")
			So(l3Rows[0].OrderID, ShouldEqual, "ORD-L3-1")
			So(l3Rows[0].Side, ShouldEqual, "bid")
			So(l3Rows[0].Event, ShouldEqual, "add")
			So(l3Rows[0].LimitPrice, ShouldEqual, 50005.0)
			So(l3Rows[0].OrderQty, ShouldEqual, 2.5)
			So(l3Rows[0].Checksum, ShouldEqual, 123456789)
		})
	})

	Convey("Given a nil catalog or nil ring", t, func() {
		Convey("Drain returns immediately on nil ring", func() {
			tables.Drain(context.Background(), nil, nil)
		})

		Convey("Drain discards ring buffer when catalog is nil until canceled", func() {
			ring := wf.NewRingBuffer[*data.Measurement[float64]](16)
			ring.Put(data.NewMeasurement[float64]("test", nil))

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})

			go func() {
				tables.Drain(ctx, nil, ring)
				close(done)
			}()

			time.Sleep(60 * time.Millisecond)
			cancel()
			<-done

			_, ok := ring.Get()
			So(ok, ShouldBeFalse)
		})
	})
}
