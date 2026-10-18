package broker

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

func TestPositionsMarshalsBothBooks(t *testing.T) {
	desk := &Desk{
		positions: &sync.Map{},
		virtuals:  &sync.Map{},
	}

	paper := NewPosition()
	paper.Symbol = "BTC/USD"
	virtual := NewPosition()
	virtual.Symbol = "ETH/USD"
	virtual.Virtual = true

	desk.positions.Store(paper.Symbol, paper)
	desk.virtuals.Store(virtual.Symbol, virtual)

	var got []Position

	if err := json.Unmarshal(desk.Positions(), &got); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("positions %d", len(got))
	}

	seen := map[string]bool{}

	for i := range got {
		seen[got[i].Symbol] = got[i].Virtual
	}

	btcVirtual, btcOK := seen["BTC/USD"]
	ethVirtual, ethOK := seen["ETH/USD"]

	if !btcOK || btcVirtual || !ethOK || !ethVirtual {
		t.Fatalf("books %#v", seen)
	}
}

func TestPositionsIncludesClosedLotsAndDeduplicates(t *testing.T) {
	desk := &Desk{
		positions: &sync.Map{},
		virtuals:  &sync.Map{},
		closed:    make([]*Position, 0),
	}

	open := NewPosition()
	open.Symbol = "BTC/USD"
	desk.positions.Store(open.Symbol, open)

	closed1 := NewPosition()
	closed1.Symbol = "SOL/USD"
	desk.closed = append(desk.closed, closed1)

	// Duplicate lot with same ID as open
	dup := NewPosition()
	dup.ID = open.ID
	dup.Symbol = open.Symbol
	desk.closed = append(desk.closed, dup)

	var got []*Position
	if err := json.Unmarshal(desk.Positions(), &got); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 positions (deduplicated), got %d", len(got))
	}

	foundSOL := false
	for _, p := range got {
		if p.Symbol == "SOL/USD" {
			foundSOL = true
		}
	}
	if !foundSOL {
		t.Fatal("expected closed position SOL/USD to be included")
	}
}

func TestBalancePublishesTheTradeValuation(t *testing.T) {
	balance := &Balance{}
	balance.wallet.Store(&kraken.Balance{
		Data: []kraken.BalanceData{{
			Asset:   "USD",
			Balance: decimal.NewFromFloat64(58.11),
		}},
	})
	unrealized := decimal.NewFromFloat64(-1.5)
	equity := decimal.NewFromFloat64(56.61)
	balance.trade.Store(&kraken.TradeBalanceResult{
		UnrealizedPnL: unrealized,
		Equity:        equity,
	})

	desk := &Desk{balance: balance}

	var got struct {
		Unrealized string `json:"n"`
		Equity     string `json:"e"`
	}

	if err := json.Unmarshal(desk.Balance(), &got); err != nil {
		t.Fatal(err)
	}

	if got.Unrealized != "-1.5" || got.Equity != "56.61" {
		t.Fatalf("valuation %+v", got)
	}
}

func TestQuoteSeedsEntryFromTheAskAndMarkFromTheBid(t *testing.T) {
	price := &Price{quotes: &sync.Map{}}
	price.SetQuote("BTC/USD", decimal.NewFromFloat64(100), decimal.NewFromFloat64(101))

	desk := &Desk{
		price:     price,
		positions: &sync.Map{},
		virtuals:  &sync.Map{},
	}

	position := NewPosition()
	position.Symbol = "BTC/USD"
	position.Qty = decimal.NewFromFloat64(2)
	desk.positions.Store(position.Symbol, position)

	var got []struct {
		Symbol     string `json:"symbol"`
		EntryPrice string `json:"entry_price"`
		Mark       string `json:"mark"`
	}

	if err := json.Unmarshal(desk.Positions(), &got); err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("positions %d", len(got))
	}

	if got[0].EntryPrice != "101" || got[0].Mark != "100" {
		t.Fatalf("quote %+v", got[0])
	}

	ask := decimal.NewFromFloat64(109)
	price.SetQuote("BTC/USD", decimal.NewFromFloat64(108), ask)

	if err := json.Unmarshal(desk.Positions(), &got); err != nil {
		t.Fatal(err)
	}

	if got[0].EntryPrice != "101" {
		t.Fatalf("entry moved to %s", got[0].EntryPrice)
	}
}
