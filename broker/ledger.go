package broker

import (
	"context"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Ledger turns trades history into holdings. Each flat-to-flat cycle on a pair is
its own position; leftover quantity is the open holding.
*/
type Ledger struct {
	tierKey   string
	price     *Price
	transport network.Client
	onSettled func(*Position)
}

/*
NewLedger wires the collaborators a restored position needs to mark and exit.
*/
func NewLedger(
	tierKey string,
	price *Price,
	transport network.Client,
	onSettled func(*Position),
) *Ledger {
	return &Ledger{
		tierKey:   tierKey,
		price:     price,
		transport: transport,
		onSettled: onSettled,
	}
}

/*
Restore pages trades history, folds it into holdings, then loads each fill's
order by ordertxid and binds it onto that holding.
*/
func (ledger *Ledger) Restore(ctx context.Context) (open []*Position, closed []*Position) {
	if ledger == nil {
		return nil, nil
	}

	open, closed = ledger.Fold(ctx, ledger.history())

	if ledger.transport == nil {
		return open, closed
	}

	if orders := NewOrders(ledger.transport); orders != nil {
		positions := make([]*Position, 0, len(open)+len(closed))
		positions = append(positions, open...)
		positions = append(positions, closed...)
		orders.Restore(positions)
	}

	return open, closed
}

func (ledger *Ledger) history() []kraken.HistoryTrade {
	if ledger == nil || ledger.transport == nil {
		return nil
	}

	seen := make(map[string]struct{})
	fills := make([]kraken.HistoryTrade, 0)
	cursor := ""

	for {
		req := kraken.NewTradesHistoryRequest(nil)
		req.Cursor = cursor

		raw, err := ledger.transport.Post(
			system.Cfg.WebSocket.Endpoints.TradesHistory,
			req,
		)

		if err != nil {
			errnie.Error(err)
			break
		}

		page := kraken.NewTradesHistoryResult(raw)

		if page == nil {
			break
		}

		for _, trade := range page.Fills() {
			if trade.ID != "" {
				if _, ok := seen[trade.ID]; ok {
					continue
				}

				seen[trade.ID] = struct{}{}
			}

			fills = append(fills, trade)
		}

		if page.Cursor.Next == "" || page.Cursor.Next == cursor {
			break
		}

		cursor = page.Cursor.Next
	}

	return fills
}

/*
Fold walks fills in time order per pair. A buy opens a holding or adds to it.
A sell peels quantity off. Flat closes that holding; the next buy starts another.
*/
func (ledger *Ledger) Fold(
	ctx context.Context, trades []kraken.HistoryTrade,
) (open []*Position, closed []*Position) {
	if ledger == nil {
		return nil, nil
	}

	grouped := make(map[string][]kraken.HistoryTrade)

	for _, trade := range trades {
		if trade.Pair == "" || trade.Volume == nil || trade.Volume.Sign() <= 0 {
			continue
		}

		symbol := ledger.symbol(trade.Pair)
		grouped[symbol] = append(grouped[symbol], trade)
	}

	for pair, rows := range grouped {
		holding, done := ledger.cycle(ctx, pair, rows)
		closed = append(closed, done...)

		if holding != nil {
			open = append(open, holding)
		}
	}

	return open, closed
}

func (ledger *Ledger) cycle(
	ctx context.Context, pair string, rows []kraken.HistoryTrade,
) (*Position, []*Position) {
	slices.SortFunc(rows, func(a, b kraken.HistoryTrade) int {
		if c := a.Time.Compare(b.Time); c != 0 {
			return c
		}

		if a.Type != b.Type {
			if a.Type == "buy" {
				return -1
			}

			return 1
		}

		return strings.Compare(a.ID, b.ID)
	})

	var current *Position
	closed := make([]*Position, 0)

	for _, trade := range rows {
		if current == nil || current.ClosedAt != nil {
			if trade.Type != "buy" {
				continue
			}

			current = NewPosition(
				ctx, pair, ledger.tierKey, "", ledger.price, ledger.transport, ledger.onSettled,
			)
			current.OpenedAt = trade.Time
			current.EntryVenueOrderID = trade.OrderID
		}

		if trade.Type == "sell" && current.ExitVenueOrderID == "" {
			current.ExitVenueOrderID = trade.OrderID
		}

		current.Apply(ledger.execution(pair, trade))

		if current.ClosedAt != nil {
			closed = append(closed, current)
		}
	}

	if current == nil || current.ClosedAt != nil ||
		current.Qty == nil || current.Qty.Sign() <= 0 {
		return nil, closed
	}

	return current, closed
}

func (ledger *Ledger) symbol(pair string) string {
	if ledger.price != nil && ledger.price.normalizer != nil {
		if name := ledger.price.normalizer.Name(pair); name != "" {
			return name
		}
	}

	return pair
}

func (ledger *Ledger) execution(symbol string, trade kraken.HistoryTrade) market.ExecutionData {
	return market.ExecutionData{
		OrderID:     trade.OrderID,
		ExecID:      trade.ID,
		ExecType:    "trade",
		Symbol:      symbol,
		Side:        trade.Type,
		LastQty:     trade.Volume,
		LastPrice:   trade.Price,
		Cost:        trade.Cost,
		OrderType:   trade.OrderType,
		Timestamp:   trade.Time,
		CumQty:      trade.Volume,
		CumCost:     trade.Cost,
		AvgPrice:    trade.Price,
		FeeUsdEquiv: trade.Fee,
	}
}
