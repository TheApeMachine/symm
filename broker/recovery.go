package broker

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
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
	instrument *Instrument
	price      *Price
	balance    *Balance
	store      *PositionStore
	positions  *sync.Map
}

/*
NewRecovery instantiates a Recovery processor with all broker dependencies.
*/
func NewRecovery(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	store *PositionStore,
	positions *sync.Map,
) *Recovery {
	ctx, cancel := context.WithCancel(ctx)

	return &Recovery{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		instrument: instrument,
		price:      price,
		balance:    balance,
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

	// The wallet is authoritative for whether a lot exists; the stored row only
	// ever carried its recorded basis. A lot with no stored row is still a real
	// lot and is adopted with the basis reconstructed from trade history — it
	// is flagged degraded so the difference is visible, never hidden.
	stored, err := recovery.store.Load(recovery.ctx, symbol, entryAt)

	if err != nil {
		return errnie.Error(err)
	}

	degraded := stored == nil

	if degraded {
		errnie.Warn("recovery: DEGRADED RECOVERY for " + symbol +
			" — no stored open position at this entry; basis reconstructed from trade history")
	}

	entryDecision, err := recovery.store.LoadOpenDecision(symbol)

	if err != nil {
		return err
	}

	position := recovery.recoveredPosition(
		pair, asset, quantity, entryPrice, entryFee, entryAt, entryPrice,
	)
	position.decisionWire = entryDecision
	position.DegradedRecovery = degraded

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

	return nil
}

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
	mark *decimal.Decimal,
) *Position {
	position := NewPosition(
		recovery.ctx, recovery.api, recovery.instrument, recovery.price,
		recovery.balance, recovery.store, pair, types.Decision{
			ID:               "recovered:" + pair.Symbol,
			ProposedQuantity: quantity,
			EntryPrice:       entryPrice,
			EntryFee:         entryFee,
			Mark:             mark,
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

	// Mark the recovered lot from the continuously-resident execution state
	// immediately, so it is valued at startup rather than left waiting for the
	// next ticker. Its own entry price seeds the mark until the first coherent
	// frame arrives, exactly as a fresh entry does.
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
