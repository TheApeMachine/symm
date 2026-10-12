package broker

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Desk owns open positions: it sizes entries from the book (exit capacity
within the slippage budget, flow noise and cash), works them in child market
orders, takes the learned exit, runs the capacity monitor, accumulates venue
fills beside their shadow fills, and reports each completed round trip.
*/
type Desk struct {
	*runtime.System
	mu        sync.Mutex
	transport Transport
	price     *Price
	balance   *Balance
	catalog   *tables.Catalog
	positions *sync.Map
	closed    []*Position
	closedMu  sync.RWMutex
	persistCh chan tables.PositionRecord
	books     *Book
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
	catalog ...*tables.Catalog,
) *Desk {
	var cat *tables.Catalog

	if len(catalog) > 0 {
		cat = catalog[0]
	}

	desk := &Desk{
		transport: transport,
		balance:   balance,
		price:     price,
		catalog:   cat,
		positions: &sync.Map{},
		closed:    make([]*Position, 0),
		persistCh: make(chan tables.PositionRecord, 500),
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)
	go desk.persistLoop()
	desk.restore()
	desk.Transition(runtime.READY)
	return desk
}

func (desk *Desk) Cash(asset string) *decimal.Decimal {
	if desk.balance == nil {
		if desk.price != nil {
			return desk.price.ReferenceCash()
		}

		return nil
	}

	return desk.balance.cash(asset)
}

func (desk *Desk) ResetPaper() error {
	desk.mu.Lock()
	desk.positions = &sync.Map{}
	desk.mu.Unlock()

	desk.closedMu.Lock()
	desk.closed = make([]*Position, 0)
	desk.closedMu.Unlock()

	if paper, ok := desk.transport.(*Paper); ok {
		if err := paper.Reset(); err != nil {
			return errnie.Error(err)
		}

		return nil
	}

	if err := ResetPaperAccount(desk.Context()); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (desk *Desk) restore() {
	desk.restoreFromCatalog()

	if desk.balance == nil || desk.balance.wallet == nil {
		return
	}

	quote := desk.balance.Quote
	if quote == "" {
		quote = "USD"
	}
	quoteAlias := krakenAssetAlias(quote)

	if paper, ok := desk.transport.(*Paper); ok {
		history, err := paper.TradesHistory()
		if err == nil && len(history.Trades) > 0 {
			desk.restoreFromHistory(history.Trades, quote)
		}
	}

	for _, entry := range desk.balance.wallet.Data {
		if entry.Asset == quote || entry.Asset == quoteAlias {
			continue
		}

		if entry.Balance == nil || entry.Balance.Sign() <= 0 {
			continue
		}

		symbol := entry.Asset + "/" + quote
		if desk.price != nil {
			symbol = desk.price.normalize(symbol)
		}

		if desk.Holds(symbol) {
			continue
		}

		desk.restoreHolding(symbol, entry.Balance)
	}
}

func (desk *Desk) restoreFromHistory(
	trades map[string]spot.Trade,
	quote string,
) {
	tradesByPair := make(map[string][]spot.Trade)

	for _, trade := range trades {
		pair := trade.Pair
		if desk.price != nil {
			pair = desk.price.normalize(pair)
		}

		if !strings.Contains(pair, "/") && strings.HasSuffix(pair, quote) {
			pair = strings.TrimSuffix(pair, quote) + "/" + quote
		}

		tradesByPair[pair] = append(tradesByPair[pair], trade)
	}

	for symbol, pairTrades := range tradesByPair {
		slices.SortFunc(pairTrades, func(left, right spot.Trade) int {
			if left.Time == nil || right.Time == nil {
				return 0
			}

			return left.Time.Cmp(right.Time)
		})

		netVolume := decimal.NewFromInt64(0)
		for _, trade := range pairTrades {
			side := strings.ToLower(trade.Type)
			if side == "buy" && trade.Volume != nil {
				netVolume = netVolume.Add(trade.Volume)
			}

			if side == "sell" && trade.Volume != nil {
				netVolume = netVolume.Sub(trade.Volume)
			}
		}

		if netVolume.Sign() <= 0 {
			continue
		}

		pos := NewPosition()
		pos.symbol = symbol

		for _, trade := range pairTrades {
			side := strings.ToLower(trade.Type)
			if side != "buy" && side != "sell" {
				continue
			}

			if pos.openedAt.IsZero() && trade.Time != nil {
				sec := int64(trade.Time.Float64())
				pos.openedAt = time.Unix(sec, 0).UTC()
			}

			if side == "buy" {
				pos.entryOrders = append(pos.entryOrders, &spot.OrderRequest{
					ClOrdId:   trade.OrderID,
					OrderType: trade.OrderType,
					Type:      trade.Type,
					Volume:    trade.Volume.String(),
				})
			}

			if side == "sell" {
				pos.exitOrders = append(pos.exitOrders, &spot.OrderRequest{
					ClOrdId:   trade.OrderID,
					OrderType: trade.OrderType,
					Type:      trade.Type,
					Volume:    trade.Volume.String(),
				})
			}

			execData := kraken.ExecutionData{
				Symbol:      symbol,
				Side:        trade.Type,
				ExecType:    "trade",
				LastQty:     trade.Volume,
				LastPrice:   trade.Price,
				Cost:        trade.Cost,
				FeeUsdEquiv: trade.Fee,
			}

			if trade.Time != nil {
				sec := int64(trade.Time.Float64())
				execData.Timestamp = time.Unix(sec, 0).UTC()
			}

			pos.Apply(execData)
		}

		if !pos.Closed() {
			desk.positions.Store(symbol, []*Position{pos})
			priceStr := "0"
			if pos.entryPrice != nil {
				priceStr = pos.entryPrice.String()
			}
			qtyStr := "0"
			if pos.qty != nil {
				qtyStr = pos.qty.String()
			}
			errnie.Info(fmt.Sprintf(
				"[desk] restored open position from history: %s %s @ %s",
				symbol, qtyStr, priceStr,
			))
		}
	}
}

func (desk *Desk) restoreFromCatalog() {
	if desk.catalog == nil {
		return
	}

	records, err := desk.catalog.Positions(desk.Context())

	if err != nil {
		errnie.Error(err)
		return
	}

	for _, rec := range records {
		if rec.Status != "closed" {
			continue
		}

		var events []LifeEvent

		if rec.Events != "" {
			_ = sonic.UnmarshalString(rec.Events, &events)
		}

		var outcome *LifeOutcome

		if rec.Outcome != "" {
			var parsedOutcome LifeOutcome

			if err := sonic.UnmarshalString(rec.Outcome, &parsedOutcome); err == nil {
				outcome = &parsedOutcome
			}
		}

		if outcome == nil {
			outcome = &LifeOutcome{
				VenueCost:     rec.VenueCost,
				VenueProceeds: rec.VenueProceeds,
				VenueFees:     rec.VenueFees,
				VenueRealized: rec.VenueRealized,
				HoldNs:        rec.HoldNs,
			}
		}

		pos := RestorePosition(
			rec.ID,
			rec.Symbol,
			rec.OpenedAt,
			rec.ClosedAt,
			events,
			outcome,
		)

		desk.closedMu.Lock()
		desk.closed = append(desk.closed, pos)
		desk.closedMu.Unlock()
	}

	if len(desk.closed) > 0 {
		errnie.Info(fmt.Sprintf("[desk] restored %d closed positions from Iceberg", len(desk.closed)))
	}
}

func (desk *Desk) restoreHolding(symbol string, balance *decimal.Decimal) {

	pos := NewPosition()
	pos.symbol = symbol
	pos.openedAt = time.Now().UTC()

	var mark *decimal.Decimal
	if desk.price != nil {
		mark = desk.price.CurrentMark(symbol)
	}

	if mark == nil || mark.Sign() <= 0 {
		mark = decimal.NewFromFloat64(1.0)
	}

	clOrdId := uuid.NewString()
	pos.entryOrders = append(pos.entryOrders, &spot.OrderRequest{
		ClOrdId:   clOrdId,
		OrderType: "market",
		Type:      "buy",
		Volume:    balance.String(),
	})

	pos.events = append(pos.events, LifeEvent{
		At:     pos.openedAt.Format(time.RFC3339),
		Kind:   "entry_order",
		Detail: fmt.Sprintf("order buy %s %s", balance.String(), symbol),
		Fields: map[string]any{
			"cl_ord_id": clOrdId,
			"symbol":    symbol,
			"volume":    balance.String(),
		},
	})

	pos.Apply(kraken.ExecutionData{
		Symbol:    symbol,
		Side:      "buy",
		ExecType:  "trade",
		LastQty:   balance,
		LastPrice: mark,
	})

	desk.positions.Store(symbol, []*Position{pos})
	errnie.Info(fmt.Sprintf(
		"[desk] restored open holding from balance: %s %s",
		symbol, balance.String(),
	))
}

/*
Enter sizes an entry from the book and the matched edge and starts working it
in child market orders. The size is the smallest of: the exit capacity within
the slippage budget at its weakest over the last expected hold, the standard
deviation of signed traded volume over one expected hold (participation
within flow noise), and what the available cash buys at the budget's entry
limit. Every refusal is an error naming its reason.
*/
func (desk *Desk) Enter(symbol string, qty *decimal.Decimal, path ...string) error {
	position := NewPosition()
	order := position.Enter(symbol, qty, path...)

	desk.mu.Lock()
	untyped, loaded := desk.positions.Load(symbol)

	if loaded && untyped != nil {
		positions := untyped.([]*Position)
		desk.positions.Store(symbol, append(positions, position))
	}

	if !loaded || untyped == nil {
		desk.positions.Store(symbol, []*Position{position})
	}
	desk.mu.Unlock()

	desk.queuePersistence(position)

	return desk.submit(symbol, order)
}

/*
Holds reports whether the symbol has a position on the desk: open, entering,
or with its exit still working.
*/
func (desk *Desk) Holds(symbol string) bool {
	untyped, exists := desk.positions.Load(symbol)

	if exists && untyped != nil {
		positions := untyped.([]*Position)

		for _, pos := range positions {
			if !pos.Closed() {
				return true
			}
		}
	}

	if desk.price != nil {
		normalized := desk.price.normalize(symbol)

		if normalized != symbol {
			untyped, exists = desk.positions.Load(normalized)

			if exists && untyped != nil {
				positions := untyped.([]*Position)

				for _, pos := range positions {
					if !pos.Closed() {
						return true
					}
				}
			}
		}
	}

	return false
}

/*
Exit takes the learned exit: it sells everything still open. It always wins:
it ends the entry plan, and when a capacity trim is in flight it sells the
rest. A position whose sell is already working is not sold twice.
*/
func (desk *Desk) Exit(symbol string) error {
	desk.mu.Lock()
	lookupSymbol := symbol
	untyped, exists := desk.positions.Load(lookupSymbol)

	if !exists && desk.price != nil {
		normalized := desk.price.normalize(symbol)
		if normalized != symbol {
			untyped, exists = desk.positions.Load(normalized)
			if exists {
				lookupSymbol = normalized
			}
		}
	}

	if !exists || untyped == nil {
		desk.mu.Unlock()
		return nil
	}

	positions := untyped.([]*Position)
	posList := slices.Clone(positions)
	desk.mu.Unlock()

	for _, position := range posList {
		if err := desk.submit(lookupSymbol, position.Exit(lookupSymbol)); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
submit writes an order to the venue transport. A nil order (nothing to do)
or an absent transport submits nothing.
*/
func (desk *Desk) submit(symbol string, order *spot.OrderRequest) error {
	if desk.transport == nil || order == nil {
		return nil
	}

	buf, err := sonic.Marshal(kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
		Pair:    symbol,
		Type:    order.Type,
		Volume:  order.Volume,
		ClOrdId: order.ClOrdId,
	}))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal, "[desk] unable to encode "+order.Type+" order for "+symbol, err,
		))
	}

	if err := desk.transport.Write(buf); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[desk] unable to submit "+order.Type+" order for "+symbol, err,
		))
	}

	return nil
}

func (desk *Desk) Apply(execution *kraken.Execution) {
	for _, data := range execution.Data {
		desk.mu.Lock()
		symbol := data.Symbol
		untyped, exists := desk.positions.Load(symbol)

		if !exists && desk.price != nil {
			normalized := desk.price.normalize(symbol)
			if normalized != symbol {
				symbol = normalized
				untyped, exists = desk.positions.Load(symbol)
			}
		}

		if !exists {
			desk.mu.Unlock()
			continue
		}

		positions := untyped.([]*Position)
		allClosed := true

		for _, position := range positions {
			position.Apply(data)

			if !position.Closed() {
				allClosed = false
			}

			if position.Closed() {
				desk.recordClosed(position)
			}
		}

		if allClosed {
			desk.positions.Delete(symbol)
		}
		desk.mu.Unlock()
	}
}

/*
Open returns a copy of the positions the desk holds for symbol.
*/
func (desk *Desk) Open(symbol string) []*Position {
	untyped, exists := desk.positions.Load(symbol)

	if !exists && desk.price != nil {
		untyped, exists = desk.positions.Load(desk.price.normalize(symbol))
	}

	if !exists || untyped == nil {
		return nil
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	return slices.Clone(untyped.([]*Position))
}

func (desk *Desk) persistLoop() {
	for {
		select {
		case <-desk.Context().Done():
			for {
				select {
				case rec := <-desk.persistCh:
					if desk.catalog != nil {
						if err := desk.catalog.RecordPosition(context.Background(), rec); err != nil {
							errnie.Error(err)
						}
					}
				default:
					return
				}
			}
		case rec, ok := <-desk.persistCh:
			if !ok {
				return
			}

			if desk.catalog != nil {
				if err := desk.catalog.RecordPosition(desk.Context(), rec); err != nil {
					errnie.Error(err)
				}
			}
		}
	}
}

func (desk *Desk) queuePersistence(pos *Position) {
	if pos == nil || desk.catalog == nil {
		return
	}

	lc := pos.Lifecycle()

	eventsJSON, err := sonic.MarshalString(lc.Events)

	if err != nil {
		eventsJSON = "[]"
	}

	var outcomeJSON string
	var venueCost float64
	var venueProceeds float64
	var venueFees float64
	var venueRealized float64
	var holdNs int64

	if lc.Outcome != nil {
		if buf, err := sonic.MarshalString(lc.Outcome); err == nil {
			outcomeJSON = buf
		}

		venueCost = lc.Outcome.VenueCost
		venueProceeds = lc.Outcome.VenueProceeds
		venueFees = lc.Outcome.VenueFees
		venueRealized = lc.Outcome.VenueRealized
		holdNs = lc.Outcome.HoldNs
	}

	rec := tables.PositionRecord{
		ID:            lc.Id,
		Symbol:        lc.Symbol,
		Status:        lc.Status,
		OpenedAt:      pos.OpenedAt(),
		ClosedAt:      pos.ClosedAt(),
		Events:        eventsJSON,
		VenueCost:     venueCost,
		VenueProceeds: venueProceeds,
		VenueFees:     venueFees,
		VenueRealized: venueRealized,
		HoldNs:        holdNs,
		Outcome:       outcomeJSON,
	}

	select {
	case desk.persistCh <- rec:
	default:
		errnie.Error(errnie.Err(
			errnie.Internal,
			"[desk] position persistence queue full for "+lc.Id,
			nil,
		))
	}
}

func (desk *Desk) recordClosed(position *Position) {
	if position == nil {
		return
	}

	desk.closedMu.Lock()

	for _, existing := range desk.closed {
		if existing.ID() == position.ID() {
			desk.closedMu.Unlock()
			return
		}
	}

	desk.closed = append(desk.closed, position)
	desk.closedMu.Unlock()

	desk.queuePersistence(position)
}

/*
Closed returns a copy of closed positions on the desk, newest first.
*/
func (desk *Desk) Closed() []*Position {
	desk.closedMu.RLock()
	defer desk.closedMu.RUnlock()

	result := slices.Clone(desk.closed)

	sort.Slice(result, func(leftIndex, rightIndex int) bool {
		return result[leftIndex].OpenedAt().After(result[rightIndex].OpenedAt())
	})

	return result
}
