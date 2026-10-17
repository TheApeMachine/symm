package hindsight

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

func TestAnnotationVectorUsesTheMarkedTrades(t *testing.T) {
	t.Parallel()

	trades := []annotationTrade{
		{tick: 10, at: 100, price: 1.0, bid: 0.9, ask: 1.1},
		{tick: 20, at: 200, price: 1.2, bid: 1.1, ask: 1.3},
		{tick: 20, at: 250, price: 1.4, bid: 1.1, ask: 1.3},
		{tick: 30, at: 300, price: 1.5, ask: 1.6},
		{tick: 40, at: 400, price: 1.1, bid: 1.0},
	}

	vector, err := annotationVector("up", [4]uint64{10, 20, 30, 40}, trades)

	if err != nil {
		t.Fatalf("vector: %v", err)
	}

	if len(vector) != len(data.ExcursionKeys) {
		t.Fatalf("len = %d", len(vector))
	}

	fields := map[string]float64{}

	for idx, key := range data.ExcursionKeys {
		fields[key] = vector[idx]
	}

	if fields["a_idx"] != 0 || fields["b_idx"] != 2 || fields["c_idx"] != 3 || fields["d_idx"] != 4 {
		t.Fatalf("indexes = a %v b %v c %v d %v", fields["a_idx"], fields["b_idx"], fields["c_idx"], fields["d_idx"])
	}

	if fields["b_price"] != 1.4 || fields["b_at"] != 250 || fields["c_price"] != 1.5 || fields["c_bid"] != 1.5 {
		t.Fatalf("prices = %+v", fields)
	}

	if _, err := annotationVector("up", [4]uint64{10, 30, 20, 40}, trades); err == nil {
		t.Fatal("out of order marks were accepted")
	}
}

func TestMarkedTradesReadTheLoadedTape(t *testing.T) {
	t.Parallel()

	print := func(tick uint64, price float64, at int64, bid, ask float64) stitch {
		return stitch{
			frame: &data.Value{Tick: tick},
			tick: &broker.SymbolTick{
				Symbol: "ETH/USD",
				Trades: []kraken.TradeData{{
					Price:     *decimal.NewFromFloat64(price),
					Timestamp: time.Unix(0, at),
				}},
				Price: broker.PriceLayer{
					Bid: decimal.NewFromFloat64(bid),
					Ask: decimal.NewFromFloat64(ask),
				},
			},
		}
	}

	tape := &runTape{bySymbol: map[string][]stitch{
		"ETH/USD": {
			print(10, 1.0, 100, 0.9, 1.1),
			print(20, 1.4, 250, 1.1, 1.3),
			print(50, 9.0, 900, 8.0, 9.1),
		},
		"BTC/USD": {
			print(20, 100.0, 250, 99.0, 101.0),
		},
	}}

	got := markedTrades(tape, "ETH/USD", 10, 40)

	if len(got) != 2 || got[0].tick != 10 || got[1].price != 1.4 || got[1].ask != 1.3 {
		t.Fatalf("trades = %+v", got)
	}
}
