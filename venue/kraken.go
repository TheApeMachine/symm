package venue

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/venue/kraken"
)

type Kraken struct {
	balance      *kraken.Balance
	tradeBalance *kraken.TradeBalance
	level3       *kraken.Level3
	trade        *kraken.Trade
	execution    *kraken.Execution
	order        *kraken.Order
	ticker       *kraken.Ticker
}

func NewKraken(_ string) *Kraken {
	return &Kraken{
		balance:      kraken.NewBalance(),
		tradeBalance: kraken.NewTradeBalance(),
		level3:       kraken.NewLevel3(),
		trade:        kraken.NewTrade(),
		execution:    kraken.NewExecution(),
		order:        kraken.NewOrder(),
		ticker:       kraken.NewTicker(),
	}
}

func (venue *Kraken) Balance() market.MarketType {
	return venue.balance
}

func (venue *Kraken) TradeBalance() market.MarketType {
	return venue.tradeBalance
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
	return venue.ticker
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
		if err := sonic.Unmarshal(message, venue.balance); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[venue.kraken] balances decode failed", err,
			))
			return nil
		}

		return venue.balance

	case head.Channel == "level3":
		if err := sonic.Unmarshal(message, venue.level3); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[venue.kraken] level3 decode failed", err,
			))
			return nil
		}

		return venue.level3

	case head.Channel == "trade":
		if err := sonic.Unmarshal(message, venue.trade); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[venue.kraken] trade decode failed", err,
			))
			return nil
		}

		return venue.trade

	case head.Channel == "executions":
		if err := sonic.Unmarshal(message, venue.execution); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[venue.kraken] executions decode failed", err,
			))
			return nil
		}

		return venue.execution

	case head.Method == "add_order":
		if err := sonic.Unmarshal(message, venue.order); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[venue.kraken] add_order decode failed", err,
			))
			return nil
		}

		return venue.order
	}

	return nil
}
