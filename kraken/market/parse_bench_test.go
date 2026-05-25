package market

import (
	"encoding/json"
	"testing"

	"github.com/theapemachine/symm/kraken/trade"
)

type bookMessage struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    []struct {
		Symbol string `json:"symbol"`
		Bids   []struct {
			Price float64 `json:"price"`
			Qty   float64 `json:"qty"`
		} `json:"bids"`
		Asks []struct {
			Price float64 `json:"price"`
			Qty   float64 `json:"qty"`
		} `json:"asks"`
	} `json:"data"`
}

func BenchmarkParseTradesEncodingJSON(b *testing.B) {
	for b.Loop() {
		var frame trade.Snapshot
		if err := json.Unmarshal(sampleTradeFrame, &frame); err != nil {
			b.Fatalf("unmarshal trades: %v", err)
		}
	}
}

func BenchmarkParseTopBookJSONParser(b *testing.B) {
	for b.Loop() {
		if _, err := ParseTopBook(sampleBookFrame); err != nil {
			b.Fatalf("parse book: %v", err)
		}
	}
}

func BenchmarkParseTopBookEncodingJSON(b *testing.B) {
	for b.Loop() {
		var message bookMessage
		if err := json.Unmarshal(sampleBookFrame, &message); err != nil {
			b.Fatalf("unmarshal book: %v", err)
		}
	}
}

func BenchmarkInstrumentMessageParse(b *testing.B) {
	for b.Loop() {
		var instrumentMessage InstrumentMessage
		if err := instrumentMessage.Parse(sampleInstrumentFrame); err != nil {
			b.Fatalf("parse instrument: %v", err)
		}
	}
}
