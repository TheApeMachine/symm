package execution

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"fmt"
	"github.com/theapemachine/symm/nomagique/runtime"
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/financial/kraken"
)

/* AccountServer owns one execution wallet, its pending orders and settled positions. */
type AccountServer struct {
	cash, initial, pnl                           *decimal.Decimal
	configuration                                string
	positions                                    map[string]*positionState
	observations, decisions, outcomes, positives uint64
	mean, m2                                     float64
	closed                                       *closedPosition
	symbol, decision, reason                     string
	moment                                       string
	epoch, sequence                              int64
	live, authorized                             bool
	revision, durableRevision                    uint64
	persistenceError                             string
	checkpoint                                   runtime.Checkpoint
	checkpointKey                                string
	saving                                       runtime.Checkpoint_save_Results_Future
	releaseSave                                  capnp.ReleaseFunc
	savingRevision                               uint64
}

/* positionState retains exact cost basis and one pending market order. */
type positionState struct {
	quantity, basis, spent, proceeds, mark                         *decimal.Decimal
	amount, reserved, fee, minimumQuantity, minimumCost, increment *decimal.Decimal
	pending                                                        string
	epoch, sequence                                                int64
	costPlaces                                                     uint32
	opened                                                         string
	orderId, clientId                                              string
	attempted                                                      bool
	filledQuantity, filledCost, filledFee                          *decimal.Decimal
	intentRevision                                                 uint64
}

type closedPosition struct {
	symbol               string
	basis, proceeds, pnl *decimal.Decimal
	edge                 float64
	epoch, sequence      int64
	opened, closed       string
}

/* NewAccount creates an idle ledger; paper capital is an explicit graph resource. */
func NewAccount() *AccountServer { return &AccountServer{positions: make(map[string]*positionState)} }

/* Write settles prior orders before selecting this observation's inventory decision. */
func (server *AccountServer) Write(ctx context.Context, call Account_write) error {
	args := call.Args()

	if !args.TermsReady() || !args.SweepReady() {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: venue terms and fill capability must activate before the ledger", nil))
	}
	market, err := args.Market()

	if err != nil {
		return errnie.Error(err)
	}
	server.moment, err = args.Time()

	if err != nil {
		return errnie.Error(err)
	}

	if _, err := time.Parse(time.RFC3339Nano, server.moment); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: observation time", err))
	}

	if !market.IsValid() || args.Epoch() <= 0 || args.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf("execution account: reconciled market and causal stamp are required (market=%t epoch=%d sequence=%d)", market.IsValid(), args.Epoch(), args.Sequence()), nil))
	}
	server.symbol, err = market.Symbol()

	if err != nil {
		return errnie.Error(err)
	}

	if server.symbol == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: market symbol is required", nil))
	}
	server.closed, server.decision, server.reason = nil, "", ""
	active, err := server.initialize(ctx, args)
	if err != nil {
		return err
	}
	if !active {
		return nil
	}
	if server.observations > 0 && (args.Epoch() < server.epoch || args.Epoch() == server.epoch && args.Sequence() <= server.sequence) {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: duplicate or regressing observation stamp", nil))
	}
	server.epoch, server.sequence = args.Epoch(), args.Sequence()
	if err := server.pollDurability(); err != nil {
		server.persistenceError = errnie.Error(err).Error()
	}
	defer func() {
		if err := server.persist(ctx); err != nil {
			server.persistenceError = errnie.Error(err).Error()
		}
	}()

	server.observations++
	position := server.positions[server.symbol]

	if server.live && position != nil && position.pending != "" {
		if err := server.reconcile(ctx, position, args); err != nil {
			server.reason = errnie.Error(err).Error()
			return nil
		}
	}
	if position != nil && market.Updated() {
		if err := server.settle(ctx, position, args); err != nil {
			return err
		}
	}

	if position != nil && position.pending != "" {
		return nil
	}
	choices, err := args.Flat()

	if err != nil {
		return errnie.Error(err)
	}

	if position != nil && position.quantity.Sign() > 0 {
		choices, err = args.Held()

		if err != nil {
			return errnie.Error(err)
		}
	}
	action, err := oneDecision(choices)

	if err != nil {
		return errnie.Error(err)
	}

	if len(action) == 0 {
		return nil
	}
	server.decision = string(action)

	if server.decision != "WAIT" && server.decision != "ENTER" && server.decision != "EXIT" {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: model emitted an unknown action", nil))
	}
	server.decisions++

	if server.decision == "WAIT" {
		return nil
	}

	if position == nil {
		position = &positionState{quantity: decimal.NewFromInt64(0), basis: decimal.NewFromInt64(0), spent: decimal.NewFromInt64(0), proceeds: decimal.NewFromInt64(0), reserved: decimal.NewFromInt64(0)}
		server.positions[server.symbol] = position
	}

	if server.decision == "ENTER" && position.quantity.Sign() > 0 || server.decision == "EXIT" && position.quantity.Sign() == 0 {
		return nil
	}
	if server.live && server.decision == "ENTER" && (!server.authorized || !args.Durable() || server.persistenceError != "" || server.durableRevision < server.revision) {
		server.reason = "live_entry_not_authorized_or_durable"
		return nil
	}
	if err := server.place(ctx, position, args); err != nil {
		return err
	}
	if position.pending != "" {
		server.revision++
		position.intentRevision = server.revision
		if server.live {
			position.clientId = server.identity()
			position.orderId = ""
			position.attempted = false
			position.filledQuantity, position.filledCost, position.filledFee = decimal.NewFromInt64(0), decimal.NewFromInt64(0), decimal.NewFromInt64(0)
		}
	}
	return nil
}

/* place reserves only a venue-minimum calibration order or exits existing inventory. */
func (server *AccountServer) place(ctx context.Context, position *positionState, args Account_write_Params) error {
	terms := args.Terms()

	if !terms.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: authoritative venue terms capability is required", nil))
	}
	future, release := terms.Quote(ctx, func(params kraken.Terms_quote_Params) error { return params.SetSymbol(server.symbol) })
	defer release()
	result, err := future.Struct()

	if err != nil {
		return errnie.Error(err)
	}
	quoted, err := result.Terms()

	if err != nil {
		return errnie.Error(err)
	}
	quotedSymbol, err := quoted.Symbol()

	if err != nil {
		return errnie.Error(err)
	}

	if quotedSymbol != server.symbol {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: venue terms refer to another market", nil))
	}
	for _, field := range []struct {
		read   func() (string, error)
		target **decimal.Decimal
		name   string
	}{
		{quoted.TakerFee, &position.fee, "fee"}, {quoted.MinimumQuantity, &position.minimumQuantity, "minimum quantity"},
		{quoted.MinimumCost, &position.minimumCost, "minimum cost"}, {quoted.QuantityIncrement, &position.increment, "quantity increment"},
	} {
		*field.target, err = amountArgument(field.read, field.name)

		if err != nil {
			return err
		}
	}

	if position.fee.Sign() < 0 || position.minimumQuantity.Sign() <= 0 || position.minimumCost.Sign() < 0 || position.increment.Sign() <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: invalid venue fee or minimum", nil))
	}
	position.costPlaces = quoted.CostPlaces()
	position.epoch, position.sequence = args.Epoch(), args.Sequence()

	if server.decision == "EXIT" {
		if position.quantity.Cmp(position.minimumQuantity) < 0 {
			server.reason = "inventory_below_venue_minimum"
			return nil
		}
		position.pending, position.amount = "sell", position.quantity.Copy()
		return nil
	}
	market, err := args.Market()

	if err != nil {
		return errnie.Error(err)
	}
	asks, err := market.Asks()

	if err != nil {
		return errnie.Error(err)
	}

	if asks.Len() == 0 {
		server.reason = "empty_ask_book"
		return nil
	}
	price, err := amountArgument(asks.At(0).Price, "best ask")

	if err != nil {
		return err
	}

	if price.Sign() <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: positive best ask is required", nil))
	}
	amount := position.minimumQuantity.Copy()
	// Round a cost-minimum quantity upward by the venue's actual lot increment.
	unitCost := price.SetScale(price.GetScale() + position.increment.GetScale()).Mul(position.increment)
	units := position.minimumCost.SetScale(max(position.minimumCost.GetScale(), unitCost.GetScale())).Div(unitCost).SetScale(0)
	needed := units.SetScale(position.increment.GetScale()).Mul(position.increment)

	if price.SetScale(price.GetScale()+needed.GetScale()).Mul(needed).Cmp(position.minimumCost) < 0 {
		needed = needed.Add(position.increment)
	}

	if needed.Cmp(amount) > 0 {
		amount = needed
	}
	quantity, cost, _, err := server.sweep(ctx, args.Sweep(), asks, amount, position.increment, false)

	if err != nil {
		return err
	}

	if quantity.Cmp(amount) < 0 {
		server.reason = "insufficient_displayed_depth"
		return nil
	}
	fee := cost.SetScale(cost.GetScale() + position.fee.GetScale()).Mul(position.fee)
	reserved := cost.SetScale(max(cost.GetScale(), fee.GetScale())).Add(fee)

	if reserved.Cmp(server.cash) > 0 {
		server.reason = "insufficient_paper_cash"
		return nil
	}
	server.cash = server.cash.SetScale(max(server.cash.GetScale(), reserved.GetScale())).Sub(reserved)
	position.reserved, position.amount, position.pending = reserved, amount, "buy"
	position.spent, position.proceeds = decimal.NewFromInt64(0), decimal.NewFromInt64(0)
	position.opened = ""
	return nil
}

/* Done publishes measured account evidence, including uncertainty rather than an invented edge threshold. */
func (server *AccountServer) Done(ctx context.Context, call Account_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	return server.emit(result)
}

/* uncertainty is the sample standard error of completed, fee-inclusive return fractions. */
func (server *AccountServer) uncertainty() float64 {
	if server.outcomes < 2 {
		return 0
	}
	return math.Sqrt(server.m2 / float64(server.outcomes-1) / float64(server.outcomes))
}
