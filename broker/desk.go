package broker

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
PositionState is the lifecycle phase of one symbol's position on the Desk.
*/
type PositionState int

const (
	FLAT PositionState = iota
	ENTERING
	HOLDING
	EXITING
)

/*
Closure is the realized outcome of one completed round trip, built from the
venue's own fill costs and fees.
*/
type Closure struct {
	Symbol   string
	Cost     *decimal.Decimal // Cumulative buy cost.
	Proceeds *decimal.Decimal // Cumulative sell proceeds.
	Fees     *decimal.Decimal // Cumulative fees across both legs.
	Realized *decimal.Decimal // Proceeds - Cost - Fees.
}

/*
position accumulates the venue's authoritative fills for one symbol.
*/
type position struct {
	bought   *decimal.Decimal
	cost     *decimal.Decimal
	sold     *decimal.Decimal
	proceeds *decimal.Decimal
	fees     *decimal.Decimal
	orders   map[string]Direction // Client order ID -> submitted side.
	exiting  bool
}

/*
Desk owns open positions: it sizes and submits entries and exits through the
transport, accumulates fills from executions, and reports the realized PnL of
each completed round trip.
*/
type Desk struct {
	*runtime.System
	transport        Transport
	price            *Price
	Balance          *Balance
	mu               sync.Mutex
	positions        map[string]*position
	positionsVersion atomic.Uint64
	closed           func(Closure)
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance ...*Balance,
) *Desk {
	desk := &Desk{
		transport: transport,
		price:     price,
		positions: make(map[string]*position),
	}

	if len(balance) > 0 {
		desk.Balance = balance[0]
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)
	desk.Transition(runtime.READY)
	return desk
}

/*
OnClose registers the consumer of realized round-trip outcomes.
*/
func (desk *Desk) OnClose(handler func(Closure)) {
	desk.closed = handler
}

/*
State reports the lifecycle phase of the symbol's position.
*/
func (desk *Desk) State(symbol string) PositionState {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	held, ok := desk.positions[symbol]

	if !ok {
		return FLAT
	}

	if held.exiting {
		return EXITING
	}

	if held.bought.Sign() <= 0 {
		return ENTERING
	}

	return HOLDING
}

/*
Enter sizes a market buy at the Price allocation budget and submits it
asynchronously so the market path never waits on the transport.
*/
func (desk *Desk) Enter(symbol string) error {
	desk.mu.Lock()

	if _, ok := desk.positions[symbol]; ok {
		desk.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[desk] position already exists for "+symbol, nil,
		))
	}

	cost, err := desk.price.AllocateEntry(symbol)

	if err != nil {
		desk.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.Validation, "[desk] unable to allocate entry for "+symbol, err,
		))
	}

	desk.positions[symbol] = &position{
		bought:   decimal.NewFromInt64(0),
		cost:     decimal.NewFromInt64(0),
		sold:     decimal.NewFromInt64(0),
		proceeds: decimal.NewFromInt64(0),
		fees:     decimal.NewFromInt64(0),
		orders:   make(map[string]Direction),
	}

	desk.positionsVersion.Add(1)
	desk.mu.Unlock()

	go desk.submit(symbol, BUY, cost.Quantity)
	return nil
}

/*
Exit submits a market sell of the full filled quantity.
*/
func (desk *Desk) Exit(symbol string) error {
	desk.mu.Lock()

	held, ok := desk.positions[symbol]

	if !ok || held.exiting || held.bought.Sign() <= 0 {
		if desk.Balance != nil {
			snap := desk.Balance.Snapshot()

			if snap != nil && snap.Assets != nil {
				asset := symbol

				if strings.Contains(symbol, "/") {
					parts := strings.Split(symbol, "/")
					asset = parts[0]
				}

				if qty, exists := snap.Assets[asset]; exists && qty != nil && qty.Sign() > 0 {
					held = &position{
						bought:   qty,
						cost:     decimal.NewFromInt64(0),
						sold:     decimal.NewFromInt64(0),
						proceeds: decimal.NewFromInt64(0),
						fees:     decimal.NewFromInt64(0),
						orders:   make(map[string]Direction),
						exiting:  true,
					}
					desk.positions[symbol] = held
					desk.positionsVersion.Add(1)
					quantity := held.bought
					desk.mu.Unlock()

					go desk.submit(symbol, SELL, quantity)
					return nil
				}
			}
		}

		desk.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[desk] no filled position to exit for "+symbol, nil,
		))
	}

	held.exiting = true
	quantity := held.bought
	desk.positionsVersion.Add(1)

	desk.mu.Unlock()

	go desk.submit(symbol, SELL, quantity)
	return nil
}

/*
Unrealized marks the open position against current bid depth, net of fees.
*/
func (desk *Desk) Unrealized(symbol string) (*decimal.Decimal, *decimal.Decimal, error) {
	desk.mu.Lock()

	held, ok := desk.positions[symbol]

	if !ok || held.bought.Sign() <= 0 {
		desk.mu.Unlock()

		return nil, nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[desk] no filled position for "+symbol, nil,
		))
	}

	quantity := held.bought
	basis := held.cost.Add(held.fees)

	desk.mu.Unlock()

	net, _, err := desk.price.Liquidate(symbol, quantity)

	if err != nil {
		return nil, nil, errnie.Error(err)
	}

	return net.Sub(basis), basis, nil
}

/*
Apply accumulates execution fills into their positions and closes a position
once the exit leg has sold everything that was bought. Fills are attributed by
the client order ID the Desk submitted, never by inferring the side.
*/
func (desk *Desk) Apply(execution *kraken.Execution) {
	if execution == nil {
		return
	}

	for _, row := range execution.Data {
		if row.LastQty == nil || row.LastQty.Sign() <= 0 {
			continue
		}

		if row.Cost == nil || row.Cost.Sign() <= 0 {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent, "[desk] fill without cost for "+row.Symbol, nil,
			))

			continue
		}

		closure, done := desk.fill(row)

		if done && desk.closed != nil {
			desk.closed(closure)
		}
	}
}

func (desk *Desk) fill(row kraken.ExecutionData) (Closure, bool) {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	held, ok := desk.positions[row.Symbol]

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound, "[desk] fill for unknown position "+row.Symbol, nil,
		))

		return Closure{}, false
	}

	side, ok := held.orders[row.ClientOrderID]

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound, "[desk] fill for unknown order "+row.ClientOrderID+" on "+row.Symbol, nil,
		))

		return Closure{}, false
	}

	if row.FeeUsdEquiv != nil {
		held.fees = held.fees.Add(row.FeeUsdEquiv)
	}

	if side == BUY {
		held.bought = held.bought.Add(row.LastQty)
		held.cost = held.cost.Add(row.Cost)
		desk.positionsVersion.Add(1)
		return Closure{}, false
	}

	held.sold = held.sold.Add(row.LastQty)
	held.proceeds = held.proceeds.Add(row.Cost)
	desk.positionsVersion.Add(1)

	if !held.exiting || held.sold.Cmp(held.bought) < 0 {
		return Closure{}, false
	}

	delete(desk.positions, row.Symbol)

	return Closure{
		Symbol:   row.Symbol,
		Cost:     held.cost,
		Proceeds: held.proceeds,
		Fees:     held.fees,
		Realized: held.proceeds.Sub(held.cost).Sub(held.fees),
	}, true
}

/*
submit writes one market order envelope to the transport. A failed entry
releases the pending position; a failed exit returns it to holding.
*/
func (desk *Desk) submit(symbol string, side Direction, quantity *decimal.Decimal) {
	clientOrderID := uuid.NewString()

	desk.mu.Lock()

	if held, ok := desk.positions[symbol]; ok {
		held.orders[clientOrderID] = side
	}

	desk.mu.Unlock()

	message := kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
		Pair:    symbol,
		Type:    string(side),
		OrdType: "market",
		Volume:  quantity.String(),
		ClOrdId: clientOrderID,
	})

	payload, err := sonic.Marshal(message)

	if err == nil {
		err = desk.transport.Write(payload)
	}

	if err == nil {
		return
	}

	errnie.Error(errnie.Err(
		errnie.IO, "[desk] order submission failed for "+symbol, err,
	))

	desk.mu.Lock()
	defer desk.mu.Unlock()

	held, ok := desk.positions[symbol]

	if !ok {
		return
	}

	if side == SELL {
		held.exiting = false
		return
	}

	if held.bought.Sign() <= 0 {
		delete(desk.positions, symbol)
	}
}

/*
PositionsVersion reports the monotonic revision of the active positions.
*/
func (desk *Desk) PositionsVersion() uint64 {
	if desk == nil {
		return 0
	}

	return desk.positionsVersion.Load()
}

/*
PositionsWire returns the active open positions and spot holdings formatted
for the telemetry websocket feed.
*/
func (desk *Desk) PositionsWire() *wire.PositionsFrameT {
	if desk == nil {
		return &wire.PositionsFrameT{Rows: []*wire.PositionT{}}
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	rows := make([]*wire.PositionT, 0)
	handled := make(map[string]bool)

	for symbol, held := range desk.positions {
		if held.bought == nil || held.bought.Sign() <= 0 {
			continue
		}

		handled[symbol] = true
		status := "holding"

		if held.exiting {
			status = "exiting"
		}

		qtyStr := held.bought.String()
		basis := held.cost.Add(held.fees)
		entryPriceStr := ""

		if held.bought.Sign() > 0 {
			entryPriceStr = basis.Div(held.bought).String()
		}

		markStr := entryPriceStr
		pnlStr := "0.00"
		returnPct := 0.0

		if desk.price != nil {
			mark := desk.price.CurrentMark(symbol)

			if mark != nil {
				markStr = mark.String()
			}

			net, _, err := desk.price.Liquidate(symbol, held.bought)

			if err == nil {
				pnl := net.Sub(basis)
				pnlStr = pnl.String()

				if basis.Sign() > 0 {
					returnPct = pnl.SetScale(decimal.DefaultScale).Div(basis).Float64() * 100
				}
			} else if mark != nil {
				proceeds := mark.Mul(held.bought)
				pnl := proceeds.Sub(basis)
				pnlStr = pnl.String()

				if basis.Sign() > 0 {
					returnPct = pnl.SetScale(decimal.DefaultScale).Div(basis).Float64() * 100
				}
			}
		}

		rows = append(rows, &wire.PositionT{
			Status: status,
			Holding: &wire.HoldingT{
				Status:      status,
				Symbol:      symbol,
				Asset:       symbol,
				Qty:         qtyStr,
				SellableQty: qtyStr,
				EntryPrice:  entryPriceStr,
				Mark:        markStr,
				Pnl:         pnlStr,
				ReturnPct:   returnPct,
			},
		})
	}

	if desk.Balance != nil {
		snap := desk.Balance.Snapshot()

		if snap != nil && snap.Wallet != nil {
			for _, item := range snap.Wallet.Data {
				if item.Asset == desk.Balance.Quote || item.Balance == nil || item.Balance.Sign() <= 0 {
					continue
				}

				symbol := item.Asset + "/" + desk.Balance.Quote

				if handled[symbol] || handled[item.Asset] {
					continue
				}

				handled[symbol] = true
				qtyStr := item.Balance.String()
				sellableStr := qtyStr

				if item.Available != nil {
					sellableStr = item.Available.String()
				}

				markStr := ""
				pnlStr := "0.00"
				returnPct := 0.0

				if desk.price != nil {
					mark := desk.price.CurrentMark(symbol)

					if mark != nil {
						markStr = mark.String()
					}
				}

				rows = append(rows, &wire.PositionT{
					Status: "holding",
					Holding: &wire.HoldingT{
						Status:      "holding",
						Symbol:      symbol,
						Asset:       item.Asset,
						Qty:         qtyStr,
						SellableQty: sellableStr,
						EntryPrice:  markStr,
						Mark:        markStr,
						Pnl:         pnlStr,
						ReturnPct:   returnPct,
					},
				})
			}
		}
	}

	return &wire.PositionsFrameT{Rows: rows}
}
