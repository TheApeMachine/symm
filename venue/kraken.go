package venue

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Kraken is the venue façade the booter wires — one persistent decoder per
channel so websocket ingress reuses buffers and OnChange reaches broker desk
types through market.MarketType accessors.
*/
type Kraken struct {
	balance       *kraken.Balance
	tradeBalance  *kraken.TradeBalance
	tradesHistory *kraken.TradesHistory
	level3        *kraken.Level3
	trade         *kraken.Trade
	execution     *kraken.Execution
	order         *kraken.Order
	ticker        *kraken.Ticker
}

/*
NewKraken assembles kraken wire types for nomagique Venue; the name argument
is reserved for multi-venue boot but Kraken is the only implementation today.
*/
func NewKraken(_ string) *Kraken {
	return &Kraken{
		balance:       kraken.NewBalance(),
		tradeBalance:  kraken.NewTradeBalance(),
		tradesHistory: kraken.NewTradesHistory(),
		level3:       kraken.NewLevel3(),
		trade:        kraken.NewTrade(),
		execution:    kraken.NewExecution(),
		order:        kraken.NewOrder(),
		ticker:       kraken.NewTicker(),
	}
}

/*
Balance exposes REST/ws cash for tier sizing — desk reads spendable spot via
ToMarket into nomagique Balance, not TradeBalance equity.
*/
func (venue *Kraken) Balance() market.MarketType {
	return venue.balance
}

/*
TradeBalance exposes private account summary for tier equity gates and hub
unrealized display; kept separate from Balance so margin fields are not spendable cash.
*/
func (venue *Kraken) TradeBalance() market.MarketType {
	return venue.tradeBalance
}

/*
TradesHistory is the REST ledger sync hook broker uses to reconcile open lots
against venue fills without trusting only the executions stream.
*/
func (venue *Kraken) TradesHistory() market.MarketType {
	return venue.tradesHistory
}

/*
Level3 is the authenticated L3 channel — broker.Book folds order-level depth
and checksum here; legacy aggregated book is not used.
*/
func (venue *Kraken) Level3() market.MarketType {
	return venue.level3
}

/*
Trade is the public trade tape for signal disruptors (CVD, toxicity) that need
prints without private auth.
*/
func (venue *Kraken) Trade() market.MarketType {
	return venue.trade
}

/*
Execution is the private fill stream wired into broker.Position and tier
pending maps; paper reuses the same envelope via kraken.Paper.
*/
func (venue *Kraken) Execution() market.MarketType {
	return venue.execution
}

/*
Order is the add_order websocket ack path — desk correlates cl_ord_id before
executions arrive.
*/
func (venue *Kraken) Order() market.MarketType {
	return venue.order
}

/*
Book returns nil because symm books on Level3 only — the ws v2 book channel
would duplicate depth without per-order state or checksum validation.
*/
func (venue *Kraken) Book() market.MarketType {
	return nil
}

/*
Ticker is REST-backed reference stats for strategy gates (volume, 24h range)
without subscribing every pair on the private socket.
*/
func (venue *Kraken) Ticker() market.MarketType {
	return venue.ticker
}

/*
Read demuxes one websocket frame by channel/method into the shared kraken
decoder that implements market.MarketType; OnChange on that instance fires
after UnmarshalJSON so broker desk wiring stays uniform across channels.
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
