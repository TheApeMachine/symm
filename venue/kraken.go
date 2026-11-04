package venue

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/venue/kraken"
)

type Kraken struct {
	balance   *kraken.Balance
	level3    *kraken.Level3
	trade     *kraken.Trade
	execution *kraken.Execution
	order     *kraken.Order
}

func NewKraken(_ string) *Kraken {
	return &Kraken{
		balance:   kraken.NewBalance(),
		level3:    kraken.NewLevel3(),
		trade:     kraken.NewTrade(),
		execution: kraken.NewExecution(),
		order:     kraken.NewOrder(),
	}
}

func (venue *Kraken) Balance() market.MarketType {
	return venue.balance
}

func (venue *Kraken) Level3() market.MarketType {
	return venue.level3
}

func (venue *Kraken) Trade() market.MarketType {
	return venue.trade
}

func (venue *Kraken) Execution() market.MarketType {
	return venue.execution
}

func (venue *Kraken) Order() market.MarketType {
	return venue.order
}

func (venue *Kraken) Book() market.MarketType {
	return nil
}

func (venue *Kraken) Ticker() market.MarketType {
	return nil
}

/*
Read demuxes a venue frame into the matching wire type. Unmarshal fires OnChange.
*/
func (venue *Kraken) Read(message []byte) market.MarketType {
	var head struct {
		Channel string `json:"channel"`
		Method  string `json:"method"`
	}

	if err := sonic.Unmarshal(message, &head); err != nil {
		return nil
	}

	switch {
	case head.Channel == "balances":
		_ = sonic.Unmarshal(message, venue.balance)
		return venue.balance

	case head.Channel == "level3":
		_ = sonic.Unmarshal(message, venue.level3)
		return venue.level3

	case head.Channel == "trade":
		_ = sonic.Unmarshal(message, venue.trade)
		return venue.trade

	case head.Channel == "executions":
		_ = sonic.Unmarshal(message, venue.execution)
		return venue.execution

	case head.Method == "add_order":
		_ = sonic.Unmarshal(message, venue.order)
		return venue.order
	}

	return nil
}
