package depthflow_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/venue/kraken"
)

type depthSnapClient struct {
	frame []byte
}

func (client *depthSnapClient) Read(...string) ([]byte, error) {
	return client.frame, nil
}

func (client *depthSnapClient) Write([]byte) error { return nil }

func (client *depthSnapClient) Post(string, any) ([]byte, error) { return nil, nil }

func (client *depthSnapClient) Watch(string, any, json.Unmarshaler) {}

func TestDepthflowStep(t *testing.T) {
	Convey("Step publishes loaded depthflow after sustained bid pressure", t, func() {
		symbol := "DF/USD"
		step := decimal.RequireFromString("1")
		snapshot := &kraken.Instrument{Channel: "instrument", Type: "snapshot"}
		snapshot.Data.Pairs = []kraken.InstrumentPair{{
			Symbol: symbol, Base: "DF", Quote: "USD", Status: "online",
			TickSize: &step, QtyMin: &step,
		}}

		frame, err := json.Marshal(snapshot)
		So(err, ShouldBeNil)

		instrument := broker.NewInstrument(context.Background(), &depthSnapClient{frame: frame})
		books := broker.NewBookManager()

		signal := depthflow.NewSignal(context.Background(), books, instrument)
		envelope := runtime.NewEnvelope(nil)
		at := time.Now().UTC()

		seedBook := func(step int) {
			bidPx := []decimal.Decimal{
				decimal.NewFromInt(100),
				decimal.NewFromInt(99),
			}
			bidQty := []decimal.Decimal{
				decimal.NewFromInt(20 + int64(step%16)),
				decimal.NewFromInt(18),
			}
			askPx := []decimal.Decimal{
				decimal.NewFromInt(101),
				decimal.NewFromInt(102),
			}
			askQty := []decimal.Decimal{
				decimal.NewFromInt(8),
				decimal.NewFromInt(6),
			}

			bids := make([]*market.Level3Order, len(bidPx))
			for i := range bids {
				bids[i] = &market.Level3Order{
					LimitPrice: &bidPx[i],
					OrderQty:   &bidQty[i],
				}
			}

			asks := make([]*market.Level3Order, len(askPx))
			for i := range asks {
				asks[i] = &market.Level3Order{
					LimitPrice: &askPx[i],
					OrderQty:   &askQty[i],
				}
			}

			books.Update(&market.Level3{
				Type: "snapshot",
				Data: []*market.Level3Data{{
					Symbol: symbol,
					Bids:   bids,
					Asks:   asks,
				}},
			})
		}

		for i := range 160 {
			seedBook(i)

			envelope.Trade = &market.Trade{Data: []*market.TradeData{{
				Symbol:    symbol,
				Side:      market.SideBuy,
				Price:     decimal.NewFromInt(100),
				Qty:       decimal.NewFromInt(3),
				Timestamp: at.Add(time.Duration(i) * time.Second),
			}}}
			envelope = signal.Step(envelope)
		}

		reading := envelope.DepthFlow[symbol]
		So(reading, ShouldNotBeNil)
		So(reading.Ready, ShouldBeTrue)
		So(int(reading.Category), ShouldEqual, 1)
		So(reading.LoadedScore, ShouldBeGreaterThan, 0.6)
		So(reading.SpoofScore, ShouldEqual, 0)
		So(reading.ThinScore, ShouldEqual, 0)
		So(reading.HypothesisSeparation, ShouldBeGreaterThan, 0.3)
	})
}
