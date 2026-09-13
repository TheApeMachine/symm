package tables

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/wf"
)

/*
Drain runs the asynchronous micro-batching drain loop against a wait-free SPSC
ring buffer. It extracts raw venue frames and pipeline measurements, routing each
record to its canonical Iceberg table family and committing snapshots at the
configured cadence.
*/
func Drain(
	ctx context.Context,
	catalog *Catalog,
	ring *wf.RingBuffer[*data.Measurement[float64]],
) {
	if ring == nil {
		return
	}

	if catalog == nil {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
				for {
					if _, ok := ring.Get(); !ok {
						break
					}
				}
			}
		}
	}

	writer := NewWriter(catalog)
	flushInterval := viper.GetDuration("hindsight.capture.flush_interval")

	if flushInterval <= 0 {
		flushInterval = 50 * time.Millisecond
	}

	commitInterval := viper.GetDuration("hindsight.capture.commit_interval")

	if commitInterval <= 0 {
		commitInterval = 30 * time.Second
	}

	commitRows := viper.GetInt("hindsight.capture.commit_rows")

	if commitRows <= 0 {
		commitRows = 20000
	}

	flushTicker := time.NewTicker(flushInterval)
	defer flushTicker.Stop()

	commitTicker := time.NewTicker(commitInterval)
	defer commitTicker.Stop()

	var tickCounter atomic.Int64
	epoch := int64(1)

	for {
		select {
		case <-ctx.Done():
			for {
				measurement, ok := ring.Get()

				if !ok {
					break
				}

				if measurement == nil {
					continue
				}

				tick := measurement.SeqIdx

				if tick <= 0 {
					tick = tickCounter.Add(1)
				}

				route(writer, tick, epoch, measurement)
			}

			if writer.Pending() > 0 {
				if err := writer.Commit(context.Background()); err != nil {
					errnie.Error(err)
				}
			}

			return

		case <-commitTicker.C:
			if writer.Pending() > 0 {
				if err := writer.Commit(ctx); err != nil {
					errnie.Error(err)
				}
			}

		case <-flushTicker.C:
			for {
				measurement, ok := ring.Get()

				if !ok {
					break
				}

				if measurement == nil {
					continue
				}

				tick := measurement.SeqIdx

				if tick <= 0 {
					tick = tickCounter.Add(1)
				}

				route(writer, tick, epoch, measurement)

				if writer.Pending() >= commitRows {
					if err := writer.Commit(ctx); err != nil {
						errnie.Error(err)
					}
				}
			}
		}
	}
}

func route(
	writer *Writer,
	tick int64,
	epoch int64,
	measurement *data.Measurement[float64],
) {
	if measurement == nil || writer == nil {
		return
	}

	channel := provenance(measurement, "channel")

	if channel == "trade" {
		if strings.HasPrefix(measurement.Source, "futures") {
			writer.AddFuturesTrade(FuturesTradeRow{
				Epoch:      epoch,
				Tick:       tick,
				Symbol:     measurement.Label,
				VenueAt:    venueAt(measurement),
				ReceivedAt: receivedAt(measurement),
				Price:      metricRaw(measurement, "price"),
				Qty:        metricRaw(measurement, "qty"),
				Side:       provenance(measurement, "side"),
				OrdType:    provenance(measurement, "ord_type"),
				TradeID:    parseTradeID(measurement),
			})

			return
		}

		writer.AddSpotTrade(SpotTradeRow{
			Epoch:      epoch,
			Tick:       tick,
			Symbol:     measurement.Label,
			VenueAt:    venueAt(measurement),
			ReceivedAt: receivedAt(measurement),
			Price:      metricRaw(measurement, "price"),
			Qty:        metricRaw(measurement, "qty"),
			Side:       provenance(measurement, "side"),
			OrdType:    provenance(measurement, "ord_type"),
			TradeID:    parseTradeID(measurement),
		})

		return
	}

	if channel == "ticker" {
		if strings.HasPrefix(measurement.Source, "futures") {
			writer.AddFuturesTicker(FuturesTickerRow{
				Epoch:        epoch,
				Tick:         tick,
				Symbol:       measurement.Label,
				VenueAt:      venueAt(measurement),
				ReceivedAt:   receivedAt(measurement),
				Bid:          metricRaw(measurement, "bid"),
				BidQty:       metricRaw(measurement, "bid_qty"),
				Ask:          metricRaw(measurement, "ask"),
				AskQty:       metricRaw(measurement, "ask_qty"),
				Last:         metricRaw(measurement, "last"),
				Volume:       metricRaw(measurement, "volume"),
				VWAP:         metricRaw(measurement, "vwap"),
				Low:          metricRaw(measurement, "low"),
				High:         metricRaw(measurement, "high"),
				Change:       metricRaw(measurement, "change"),
				ChangePct:    metricRaw(measurement, "change_pct"),
				MarkPrice:    metricRaw(measurement, "mark_price"),
				IndexPrice:   metricRaw(measurement, "index_price"),
				OpenInterest: metricRaw(measurement, "open_interest"),
			})

			return
		}

		writer.AddSpotTicker(SpotTickerRow{
			Epoch:      epoch,
			Tick:       tick,
			Symbol:     measurement.Label,
			VenueAt:    venueAt(measurement),
			ReceivedAt: receivedAt(measurement),
			Bid:        metricRaw(measurement, "bid"),
			BidQty:     metricRaw(measurement, "bid_qty"),
			Ask:        metricRaw(measurement, "ask"),
			AskQty:     metricRaw(measurement, "ask_qty"),
			Last:       metricRaw(measurement, "last"),
			Volume:     metricRaw(measurement, "volume"),
			VWAP:       metricRaw(measurement, "vwap"),
			Low:        metricRaw(measurement, "low"),
			High:       metricRaw(measurement, "high"),
			Change:     metricRaw(measurement, "change"),
			ChangePct:  metricRaw(measurement, "change_pct"),
		})

		return
	}

	if channel == "level3" {
		limitPrice := metricRaw(measurement, "limit_price")

		if limitPrice == 0 {
			limitPrice = metricRaw(measurement, "price")
		}

		orderQty := metricRaw(measurement, "order_qty")

		if orderQty == 0 {
			orderQty = metricRaw(measurement, "qty")
		}

		writer.AddSpotLevel3(SpotLevel3Row{
			Epoch:      epoch,
			Tick:       tick,
			Symbol:     measurement.Label,
			VenueAt:    venueAt(measurement),
			ReceivedAt: receivedAt(measurement),
			Side:       provenance(measurement, "side"),
			Event:      provenance(measurement, "event"),
			OrderID:    provenance(measurement, "order_id"),
			LimitPrice: limitPrice,
			OrderQty:   orderQty,
			Checksum:   int64(metricRaw(measurement, "checksum")),
		})

		return
	}

	if channel == "futures.trade" {
		ordType := provenance(measurement, "type")

		if ordType == "" {
			ordType = provenance(measurement, "ord_type")
		}

		writer.AddFuturesTrade(FuturesTradeRow{
			Epoch:      epoch,
			Tick:       tick,
			Symbol:     measurement.Label,
			VenueAt:    venueAt(measurement),
			ReceivedAt: receivedAt(measurement),
			Price:      metricRaw(measurement, "price"),
			Qty:        metricRaw(measurement, "qty"),
			Side:       provenance(measurement, "side"),
			OrdType:    ordType,
			TradeID:    parseTradeID(measurement),
		})

		return
	}

	if channel == "futures.ticker" {
		writer.AddFuturesTicker(FuturesTickerRow{
			Epoch:        epoch,
			Tick:         tick,
			Symbol:       measurement.Label,
			VenueAt:      venueAt(measurement),
			ReceivedAt:   receivedAt(measurement),
			Bid:          metricRaw(measurement, "bid"),
			BidQty:       metricRaw(measurement, "bid_qty"),
			Ask:          metricRaw(measurement, "ask"),
			AskQty:       metricRaw(measurement, "ask_qty"),
			Last:         metricRaw(measurement, "last"),
			Volume:       metricRaw(measurement, "volume"),
			VWAP:         metricRaw(measurement, "vwap"),
			Low:          metricRaw(measurement, "low"),
			High:         metricRaw(measurement, "high"),
			Change:       metricRaw(measurement, "change"),
			ChangePct:    metricRaw(measurement, "change_pct"),
			MarkPrice:    metricRaw(measurement, "mark_price"),
			IndexPrice:   metricRaw(measurement, "index_price"),
			OpenInterest: metricRaw(measurement, "open_interest"),
		})

		return
	}

	if channel == "executions" {
		orderUserRef := int64(metricRaw(measurement, "order_userref"))

		if orderUserRef == 0 {
			if refStr := provenance(measurement, "order_userref"); refStr != "" {
				if parsed, err := strconv.ParseInt(refStr, 10, 64); err == nil {
					orderUserRef = parsed
				}
			}
		}

		writer.AddExecution(ExecutionRow{
			Epoch:        epoch,
			Tick:         tick,
			Symbol:       measurement.Label,
			VenueAt:      venueAt(measurement),
			ReceivedAt:   receivedAt(measurement),
			OrderID:      provenance(measurement, "order_id"),
			OrderUserRef: orderUserRef,
			ExecID:       provenance(measurement, "exec_id"),
			ExecType:     provenance(measurement, "exec_type"),
			TradeID:      parseTradeID(measurement),
			Side:         provenance(measurement, "side"),
			LastQty:      metricExact(measurement, "last_qty"),
			LastPrice:    metricExact(measurement, "last_price"),
			LiquidityInd: provenance(measurement, "liquidity_ind"),
			Cost:         metricExact(measurement, "cost"),
			OrderType:    provenance(measurement, "order_type"),
			OrderStatus:  provenance(measurement, "order_status"),
			CumQty:       metricExact(measurement, "cum_qty"),
			CumCost:      metricExact(measurement, "cum_cost"),
			AvgPrice:     metricExact(measurement, "avg_price"),
			FeeUsdEquiv:  metricExact(measurement, "fee_usd_equiv"),
			Fees:         provenance(measurement, "fees"),
		})

		return
	}

	metricsMap := make(map[string]float64, len(measurement.Metrics))

	for key, value := range measurement.Metrics {
		metricsMap[key] = value.Raw
	}

	metadataMap := make(map[string]float64)

	for key, value := range measurement.Metadata {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			metadataMap[key] = floatVal
		}
	}

	writer.AddMeasurement(MeasurementRow{
		Epoch:      epoch,
		Tick:       tick,
		Source:     measurement.Source,
		Symbol:     measurement.Label,
		VenueAt:    venueAt(measurement),
		ObservedAt: receivedAt(measurement),
		Maturity:   measurement.Maturity,
		SNR:        measurement.SNR,
		SNRDefined: measurement.SNRDefined,
		Metrics:    metricsMap,
		Metadata:   metadataMap,
	})
}

func venueAt(measurement *data.Measurement[float64]) time.Time {
	if !measurement.From.IsZero() {
		return measurement.From
	}

	if !measurement.At.IsZero() {
		return measurement.At
	}

	return time.Now()
}

func receivedAt(measurement *data.Measurement[float64]) time.Time {
	if !measurement.At.IsZero() {
		return measurement.At
	}

	return time.Now()
}

func metricRaw(measurement *data.Measurement[float64], key string) float64 {
	if measurement.Metrics == nil {
		return 0
	}

	return measurement.Metrics[key].Raw
}

func metricExact(measurement *data.Measurement[float64], key string) *decimal.Decimal {
	if measurement.Metrics == nil {
		return nil
	}

	return measurement.Metrics[key].Exact
}

func provenance(measurement *data.Measurement[float64], key string) string {
	if measurement.Provenance == nil {
		return ""
	}

	return measurement.Provenance[key]
}

func parseTradeID(measurement *data.Measurement[float64]) int64 {
	tradeID := int64(metricRaw(measurement, "trade_id"))

	if tradeID != 0 {
		return tradeID
	}

	val := provenance(measurement, "trade_id")

	if val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			return parsed
		}
	}

	return 0
}
