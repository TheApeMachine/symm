package broker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
)

func TestPriceCaptureFeeProfiles(t *testing.T) {
	Convey("Given active venue fees and pair metadata", t, func() {
		path := filepath.Join(t.TempDir(), "market-frames.jsonl")
		recorder, err := audit.NewRecorder(path)
		So(err, ShouldBeNil)
		taker := kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}
		maker := kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.16)}
		price := newTradeVolumePrice(t, &kraken.TradeVolumeResult{
			Fees:      map[string]kraken.TradeVolumeFee{"XXBTZUSD": taker},
			FeesMaker: map[string]kraken.TradeVolumeFee{"XXBTZUSD": maker},
		})
		price.capture = recorder

		So(price.GetFees([]string{"BTC/USD"}), ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)
		file, err := os.Open(path)
		So(err, ShouldBeNil)
		defer file.Close()
		var frame struct {
			Endpoint string `json:"endpoint"`
			Payload  struct {
				Channel string                 `json:"channel"`
				Type    string                 `json:"type"`
				Data    []kraken.MarketProfile `json:"data"`
			} `json:"payload"`
		}
		So(json.NewDecoder(file).Decode(&frame), ShouldBeNil)

		Convey("It should record the exact symbol mapping and both active tiers", func() {
			So(frame.Endpoint, ShouldEqual, "symm_metadata")
			So(frame.Payload.Channel, ShouldEqual, "symm_metadata")
			So(frame.Payload.Type, ShouldEqual, "market_profiles")
			So(frame.Payload.Data, ShouldHaveLength, 1)
			So(frame.Payload.Data[0].Symbol, ShouldEqual, "BTC/USD")
			So(frame.Payload.Data[0].Pair.AltName, ShouldEqual, "XBTUSD")
			So(frame.Payload.Data[0].Taker.Fee.String(), ShouldEqual, "0.26")
			So(frame.Payload.Data[0].Maker.Fee.String(), ShouldEqual, "0.16")
		})
	})
}

func BenchmarkPriceCaptureFeeProfiles(b *testing.B) {
	recorder, err := audit.NewRecorder(filepath.Join(b.TempDir(), "market-frames.jsonl"))

	if err != nil {
		b.Fatal(err)
	}

	defer recorder.Close()
	taker := kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}
	maker := kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.16)}
	result := &kraken.TradeVolumeResult{
		Fees:      map[string]kraken.TradeVolumeFee{"XXBTZUSD": taker},
		FeesMaker: map[string]kraken.TradeVolumeFee{"XXBTZUSD": maker},
	}
	price := newTradeVolumePrice(b, result)
	price.capture = recorder
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err = price.captureFeeProfiles([]string{"BTC/USD"}, result); err != nil {
			b.Fatal(err)
		}
	}
}
