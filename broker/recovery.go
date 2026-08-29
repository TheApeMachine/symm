package broker

import (
	"context"
	"errors"
	"github.com/theapemachine/symm/nomagique/runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Recovery reconstructs active positions and working orders from exchange state on boot.
*/
type Recovery struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	bus        *runtime.Workspace
	instrument *Instrument
	price      *Price
	balance    *Balance
	recorder   *audit.Recorder
	store      *PositionStore
	positions  *sync.Map
}

/*
NewRecovery instantiates a Recovery processor with all broker dependencies.
*/
func NewRecovery(
	ctx context.Context,
	api *websocket.API,
	bus *runtime.Workspace,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	recorder *audit.Recorder,
	store *PositionStore,
	positions *sync.Map,
) *Recovery {
	ctx, cancel := context.WithCancel(ctx)

	return &Recovery{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		bus:        bus,
		instrument: instrument,
		price:      price,
		balance:    balance,
		recorder:   recorder,
		store:      store,
		positions:  positions,
	}
}

/*
Recover discovers open inventory, reconciles entry basis from fill history,
and adopts working exits.
*/
func (recovery *Recovery) Recover() error {
	balances, err := recovery.api.Balance()

	if err != nil {
		return errnie.Error(err)
	}

	history, err := recovery.api.TradesHistory()

	if err != nil {
		return errnie.Error(err)
	}

	working, err := recovery.api.OpenOrders()

	if err != nil {
		return errnie.Error(err)
	}

	if err := recovery.cancelBuys(working.Open); err != nil {
		return err
	}

	quote := recovery.api.Normalizer().Name(viper.GetString("market.quote_currency"))

	// Recovery runs before the paced instrument subscription loads fee
	// tiers (that happens much later in boot, batched across the whole
	// tradeable universe), so a wallet balance needing synthesized
	// protection would otherwise find no fee for its symbol and fail purely
	// on timing. Fetching fees for just the held symbols here, up front,
	// removes that ordering dependency without waiting on the full universe.
	var recoverySymbols []string

	for asset, amount := range balances {
		asset = recovery.api.Normalizer().Name(asset)

		if asset == "" || asset == quote || amount == nil || amount.Sign() <= 0 {
			continue
		}

		recoverySymbols = append(recoverySymbols, asset+"/"+quote)
	}

	if len(recoverySymbols) > 0 {
		if err := recovery.price.GetFees(recoverySymbols); err != nil {
			errnie.Warn("recovery: failed to preload fee tiers for held symbols: " + err.Error())
		}
	}

	// One asset's recovery failure must not cost every other asset its
	// tracking and protection: balances iterates in random map order, so
	// returning on the first error here silently orphaned whichever assets
	// hadn't been visited yet — genuinely open, unprotected, invisible in the
	// UI, with no boot-time signal beyond a log line nothing read. Every
	// asset is now attempted regardless of an earlier failure, and every
	// failure is collected and reported once recovery finishes attempting
	// the whole balance sheet.
	var recoveryErrors []error

	for asset, amount := range balances {
		asset = recovery.api.Normalizer().Name(asset)

		if asset == "" || asset == quote || amount == nil || amount.Sign() <= 0 {
			continue
		}

		if err := recovery.recoverAsset(
			asset, quote, amount, history.Trades, working.Open,
		); err != nil {
			recoveryErrors = append(recoveryErrors, errnie.Error(errnie.Err(
				errnie.Internal,
				"recovery: failed to recover "+asset+"/"+quote,
				err,
			)))
		}
	}

	if len(recoveryErrors) > 0 {
		return errors.Join(recoveryErrors...)
	}

	for orderID, order := range working.Open {
		if order.Description != nil && strings.EqualFold(order.Description.Type, "sell") {
			return errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: working sell "+orderID+" has no reconciled wallet inventory",
				nil,
			))
		}
	}

	return nil
}

func (recovery *Recovery) cancelBuys(orders map[string]spot.Order) error {
	var cancelErrors []error
	canceled := false

	for orderID, order := range orders {
		if order.Description == nil || !strings.EqualFold(order.Description.Type, "buy") {
			continue
		}

		if _, err := uuid.Parse(order.ClOrdID); err != nil {
			cancelErrors = append(cancelErrors, errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: working buy "+orderID+" is not identifiable as a symm order",
				nil,
			)))

			continue
		}

		result, err := recovery.api.CancelOrder(&spot.CancelOrderRequest{TxID: orderID})

		if err != nil {
			cancelErrors = append(cancelErrors, errnie.Error(err))

			continue
		}

		if result.Count <= 0 && !result.Pending {
			cancelErrors = append(cancelErrors, errnie.Error(errnie.Err(
				errnie.NotFound,
				"desk: working entry "+orderID+" could not be canceled",
				nil,
			)))

			continue
		}

		canceled = true
	}

	if len(cancelErrors) > 0 {
		return errors.Join(cancelErrors...)
	}

	if canceled {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: canceled working entries; restart after they reach a terminal state",
			nil,
		))
	}

	return nil
}

func (recovery *Recovery) recoverAsset(
	asset string,
	quote string,
	amount *decimal.Decimal,
	history map[string]spot.Trade,
	working map[string]spot.Order,
) error {
	symbol := asset + "/" + quote
	pair := recovery.instrument.Pair(symbol)

	if pair.Symbol == "" {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"recovery: no instrument pair for "+symbol+"; balance "+amount.String()+" cannot be recovered",
			nil,
		))
	}

	orderID, order, err := recovery.recoveredSell(pair, working)

	if err != nil {
		return err
	}

	quantity, err := recovery.api.Normalizer().FormatSize(symbol, amount)

	if err != nil {
		return errnie.Error(err)
	}

	entryPrice, entryFee, entryAt, hasFullClose := recovery.recoverBasis(pair, history)

	if entryPrice == nil && order == nil {
		// recoverBasis found no open basis. If it genuinely observed this
		// symbol's ledger go to zero at least once, the wallet's remaining
		// balance is confirmed-closed dust — venue settlement leaves its own
		// residue independent of what the reported trade rows sum to, so
		// comparing exact quantities here would be the wrong tool. Anything
		// else — no trade history at all for a symbol the wallet holds — is
		// a real balance this recovery cannot account for, which must fail
		// loudly instead of being skipped.
		if hasFullClose {
			return nil
		}

		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"recovery: wallet balance "+amount.String()+" for "+symbol+" is not accounted for by trade history",
			nil,
		))
	}

	if quantity == nil || quantity.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"recovery: normalized size is non-positive for "+symbol,
			nil,
		))
	}

	if entryPrice == nil || entryPrice.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"recovery: entry basis required for "+symbol,
			nil,
		))
	}

	stoploss, err := recovery.store.Load(recovery.ctx, symbol, entryAt)

	if err != nil {
		return errnie.Error(err)
	}

	if stoploss == nil {
		stoploss, err = recovery.synthesizeStoploss(pair, symbol, quantity, entryPrice, entryAt)

		if err != nil {
			return err
		}

		errnie.Warn("recovery: no stored stoploss for " + symbol + " at this entry; rebuilt protection from current market")
	}

	position := recovery.recoveredPosition(
		pair, asset, quantity, entryPrice, entryFee, entryAt, stoploss,
	)

	if order != nil {
		position.ExitOrder = &spot.AddOrderRequest{
			ClOrdId: order.ClOrdID,
			Type:    "sell",
			Volume:  quantity.String(),
			Pair:    symbol,
		}
		position.setStatus(types.PENDING)
		position.Holding.Status = types.PENDING
		delete(working, orderID)
	}

	position.Publish()
	return nil
}

/*
synthesizeStoploss rebuilds protection for a position whose wallet inventory
and trade history are intact but whose stored stoploss row is gone (the
process died between a fill landing and its execution frame persisting one,
or the row was otherwise lost). It mirrors the desk's live entry construction
in NewDesk's admission path: same NewRiskPlan with DefaultRiskMultiples, same
NewStoplossWithPlan call. Horizon is forced to 0 so Reconsider can never
expire a forecast this recovered lot never had.

Recovery runs before the instrument subscription has delivered any ticker or
book frame — there is no live quote to price a spread or market-impact band
from yet, only the wallet balance and trade history. NewRiskPlan already
falls back to a tick-granularity noise band when spread and impact are absent
(the standard "no book yet" case, not a recovery-specific concession), so this
prices the plan directly off entryPrice and the venue's own tick size rather
than requiring a live EntryCost read. The lot's mark starts at its own entry
price for the same reason: the desk's own evaluateExecutable call right after
construction (recovery.go's recoveredPosition) supplies the real mark as soon
as the first coherent tick or L3 frame arrives, exactly as a fresh entry does
before its own first tick.

The wallet is authoritative for whether a position exists; the stoploss store
only ever supplied its protection parameters. Refusing to track a real
position for want of that row leaves it live and completely unprotected, which
is strictly worse than a hard-floor stop rebuilt from known entry economics.
*/
func (recovery *Recovery) synthesizeStoploss(
	pair kraken.InstrumentPair,
	symbol string,
	quantity *decimal.Decimal,
	entryPrice *decimal.Decimal,
	entryAt time.Time,
) (*types.Stoploss, error) {
	if pair.TickSize.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"recovery: positive tick size required to rebuild protection for "+symbol,
			nil,
		))
	}

	fee := recovery.price.Fee(symbol)

	if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"recovery: valid taker fee required to rebuild protection for "+symbol,
			nil,
		))
	}

	feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(decimal.NewFromInt64(100))

	// A live book is a bonus when it happens to already be available, not a
	// requirement: its spread/impact only sharpen the noise band NewRiskPlan
	// would otherwise floor at venue tick granularity.
	var spread, impact *decimal.Decimal
	mark := entryPrice

	if cost, err := recovery.price.EntryCost(symbol, quantity); err == nil {
		spread = cost.Spread
		impact = cost.Impact
		mark = cost.BestBid
	}

	plan := types.NewRiskPlan(types.RiskInputs{
		ReferencePrice: entryPrice,
		Spread:         spread,
		Impact:         impact,
		TickSize:       &pair.TickSize,
		ExitFeeRate:    feeRate,
		EntryFeeRate:   feeRate,
		MaxLoss:        decimal.NewFromInt64(0).Add(entryPrice).Mul(quantity),
		Multiples:      types.DefaultRiskMultiples(),
	})

	if !plan.Present {
		return nil, errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"recovery: current execution geometry cannot support rebuilt protection for "+symbol,
			nil,
		))
	}

	stoploss, err := types.NewStoplossWithPlan(
		recovery.ctx,
		symbol,
		entryPrice,
		mark,
		nil,
		0,
		&pair.TickSize,
		feeRate,
		feeRate,
		&plan,
		entryAt,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"recovery: could not rebuild protection for "+symbol,
			err,
		))
	}

	return stoploss, nil
}

/*
recoverBasis nets one symbol's trade history into a per-unit entry price. The
fourth return value, hasFullClose, is true when a sell fully or over-closed
the tracked buy quantity at least once: proof this symbol's ledger genuinely
went to zero, not merely that the running total happens to be small. Recovery
uses it to tell a confirmed-closed wallet remainder (whatever dust the venue's
own settlement leaves behind after a real round trip) apart from a balance
this trade history simply never explains.
*/
func (recovery *Recovery) recoverBasis(
	pair kraken.InstrumentPair,
	history map[string]spot.Trade,
) (*decimal.Decimal, *decimal.Decimal, time.Time, bool) {
	trades := make([]spot.Trade, 0)
	venue := strings.ToUpper(pair.Base + pair.Quote)

	for _, trade := range history {
		if strings.ToUpper(strings.ReplaceAll(trade.Pair, "/", "")) == venue {
			trades = append(trades, trade)
		}
	}

	if len(trades) == 0 {
		return nil, decimal.NewFromInt64(0), time.Time{}, false
	}

	sort.Slice(trades, func(left, right int) bool {
		return trades[left].Time.Cmp(trades[right].Time) < 0
	})

	quantity := decimal.NewFromInt64(0)
	cost := decimal.NewFromInt64(0)
	fee := decimal.NewFromInt64(0)
	entryAt := time.Time{}
	hasFullClose := false

	for _, trade := range trades {
		if trade.Volume == nil || trade.Cost == nil || trade.Time == nil {
			continue
		}

		if strings.EqualFold(trade.Type, "buy") {
			if quantity.Sign() == 0 {
				entryAt = time.Unix(trade.Time.Int64(), 0).UTC()
			}

			quantity = addAmount(quantity, trade.Volume)
			cost = addAmount(cost, trade.Cost)

			if trade.Fee != nil {
				fee = addAmount(fee, trade.Fee)
			}

			continue
		}

		if strings.EqualFold(trade.Type, "sell") {
			if trade.Volume.Cmp(quantity) >= 0 {
				quantity = decimal.NewFromInt64(0)
				cost = decimal.NewFromInt64(0)
				fee = decimal.NewFromInt64(0)
				entryAt = time.Time{}
				hasFullClose = true
				continue
			}

			remaining := subtractAmount(quantity, trade.Volume)
			cost = cost.Mul(remaining).Div(quantity)
			fee = fee.Mul(remaining).Div(quantity)
			quantity = remaining
		}
	}

	if quantity.Sign() > 0 && cost.Sign() > 0 {
		return cost.Div(quantity), fee, entryAt, hasFullClose
	}

	return nil, decimal.NewFromInt64(0), time.Time{}, hasFullClose
}

func (recovery *Recovery) recoveredPosition(
	pair kraken.InstrumentPair,
	asset string,
	quantity *decimal.Decimal,
	entryPrice *decimal.Decimal,
	entryFee *decimal.Decimal,
	entryAt time.Time,
	stoploss *types.Stoploss,
) *Position {
	position := NewPosition(
		recovery.ctx, recovery.api, recovery.bus, recovery.instrument, recovery.price,
		recovery.balance, recovery.recorder, recovery.store, pair, types.Decision{
			ID:               "recovered:" + pair.Symbol,
			ProposedQuantity: quantity,
			EntryPrice:       entryPrice,
			EntryFee:         entryFee,
			Mark:             stoploss.Mark,
			Stoploss:         stoploss,
		},
	)

	position.setStatus(types.OPEN)
	position.Holding.Asset = asset
	position.Holding.Qty = quantity
	position.Holding.SellableQty = quantity
	position.Holding.EntryAt = &entryAt
	position.Holding.EntryFee = entryFee
	position.Holding.Status = types.OPEN
	position.onClose = func() {
		recovery.positions.CompareAndDelete(pair.Symbol, position)
	}
	recovery.positions.Store(pair.Symbol, position)

	// Restore persisted protection and immediately derive the current
	// executable state from the authoritative book, so a recovered exposed
	// position is evaluated at startup rather than left waiting for the next
	// ticker. During clean bootstrap the book is not yet valid and ObserveExecutable
	// stays armed; a valid feed that has already diverged surfaces execution
	// risk immediately. No magic bootstrap timeout is introduced.
	position.evaluateExecutable(pair.Symbol, time.Now())

	return position
}

func (recovery *Recovery) recoveredSell(
	pair kraken.InstrumentPair,
	orders map[string]spot.Order,
) (string, *spot.Order, error) {
	venue := strings.ToUpper(pair.Base + pair.Quote)
	orderID := ""
	var recovered *spot.Order

	for candidateID, order := range orders {
		if order.Description == nil || !strings.EqualFold(order.Description.Type, "sell") {
			continue
		}

		ordered := strings.ToUpper(strings.ReplaceAll(order.Description.Pair, "/", ""))

		if ordered != venue {
			continue
		}

		if recovered != nil {
			return "", nil, errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: multiple working sells exist for "+pair.Symbol,
				nil,
			))
		}

		copy := order
		orderID = candidateID
		recovered = &copy
	}

	return orderID, recovered, nil
}

func addAmount(left, right *decimal.Decimal) *decimal.Decimal {
	return decimal.NewFromInt64(0).Add(left).Add(right)
}

func subtractAmount(left, right *decimal.Decimal) *decimal.Decimal {
	return decimal.NewFromInt64(0).Add(left).Sub(right)
}
