package paper

import (
	"bytes"
	"encoding/json"
	"math/big"
	"sort"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

const (
	actionEnter = "ENTER"
	actionExit  = "EXIT"
	actionWait  = "WAIT"

	// feeWindow is the exchange's trailing volume period for fee tiers.
	feeWindow = 30 * 24 * time.Hour

	// Reasons a decision placed nothing. Holding, flat and pending are the
	// account's own state; the others mean the exchange would not quote it.
	reasonHolding  = "holding"
	reasonFlat     = "flat"
	reasonPending  = "pending"
	reasonCurrency = "currency"
	reasonUnpriced = "unknown_pair"
)

type holding struct {
	quantity *decimal.Decimal
	basis    *decimal.Decimal // cost and fees of what is still held
	spent    *decimal.Decimal // cost and fees of everything bought
	proceeds *decimal.Decimal // net of fees, of everything sold
	opened   string
}

type outstanding struct {
	symbol   string
	side     string
	reserved *decimal.Decimal
}

type traded struct {
	at     time.Time
	volume *decimal.Decimal
}

type closure struct {
	symbol               string
	opened, closed       string
	basis, proceeds, pnl *decimal.Decimal
	ratio                float64
}

type refusal struct{ symbol, action, reason string }

type outcome struct {
	closed  *closure
	refused *refusal
}

/* terms are the account's standing instructions: its currency and order size. */
type terms struct {
	currency string
	fraction *decimal.Decimal
}

/* account owns cash, holdings, orders in flight and the volume that sets its fee. */
type account struct {
	cash     *decimal.Decimal
	holdings map[string]*holding
	orders   map[uint64]outstanding
	volume   []traded
	pairs    map[string]spot.AssetPair
	schedule []byte
	issued   uint64
	outcomes []outcome
	now      time.Time
}

func newAccount() *account {
	return &account{
		holdings: make(map[string]*holding),
		orders:   make(map[uint64]outstanding),
		pairs:    make(map[string]spot.AssetPair),
	}
}

/* open funds the account once and re-reads the fee schedule when it changes. */
func (wallet *account) open(capital string, schedule []byte) error {
	if wallet.cash == nil {
		opening, err := amount(capital, "capital")

		if err != nil {
			return err
		}
		wallet.cash = opening
	}

	if bytes.Equal(schedule, wallet.schedule) {
		return nil
	}
	var table map[string]spot.AssetPair

	if err := json.Unmarshal(schedule, &table); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "account: decode fee schedule", err))
	}
	pairs := make(map[string]spot.AssetPair, len(table))

	for _, pair := range table {
		if pair.WSName == "" || len(pair.Fees) == 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "account: fee schedule entry needs wsname and fees", nil))
		}
		pairs[pair.WSName] = pair
	}
	wallet.pairs = pairs
	wallet.schedule = bytes.Clone(schedule)
	return nil
}

/* decide turns a decision into an order, or records why it placed nothing. */
func (wallet *account) decide(symbol, action, moment string, standing terms) (*order, error) {
	if symbol == "" {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "account: a decision needs a symbol", nil))
	}

	if err := wallet.advance(moment); err != nil {
		return nil, err
	}

	switch action {
	case actionWait:
		return nil, nil
	case actionEnter:
		return wallet.enter(symbol, standing)
	case actionExit:
		return wallet.exit(symbol)
	}
	return nil, errnie.Error(errnie.Err(errnie.Validation, "account: unknown action "+action, nil))
}

/* advance moves tape time forward and forgets volume outside the fee window. */
func (wallet *account) advance(moment string) error {
	at, err := time.Parse(time.RFC3339Nano, moment)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "account: time is not RFC 3339", err))
	}

	if at.After(wallet.now) {
		wallet.now = at
	}
	kept := wallet.volume[:0]

	for _, trade := range wallet.volume {
		if wallet.now.Sub(trade.at) < feeWindow {
			kept = append(kept, trade)
		}
	}
	wallet.volume = kept
	return nil
}

func (wallet *account) traded() *decimal.Decimal {
	total := zero()

	for _, trade := range wallet.volume {
		total = plus(total, trade.volume)
	}
	return total
}

/* rate is the taker fee fraction the account's trailing volume has reached. */
func (wallet *account) rate(pair spot.AssetPair) (*decimal.Decimal, error) {
	volume := wallet.traded()
	reached := pair.Fees[0].Percent

	for _, tier := range pair.Fees {
		if tier.Volume.Cmp(volume) <= 0 {
			reached = tier.Percent
		}
	}
	return percent(reached)
}

func (wallet *account) available() *decimal.Decimal {
	free := wallet.cash

	for _, placed := range wallet.orders {
		free = minus(free, placed.reserved)
	}
	return free
}

func (wallet *account) pending(symbol string) bool {
	for _, placed := range wallet.orders {
		if placed.symbol == symbol {
			return true
		}
	}
	return false
}

func (wallet *account) held() []string {
	symbols := make([]string, 0, len(wallet.holdings))

	for symbol := range wallet.holdings {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return symbols
}

func (wallet *account) refuse(symbol, action, reason string) (*order, error) {
	wallet.outcomes = append(wallet.outcomes, outcome{refused: &refusal{symbol, action, reason}})
	return nil, nil
}

/* enter spends fraction of the available cash, fee included, on symbol. */
func (wallet *account) enter(symbol string, standing terms) (*order, error) {
	if _, holds := wallet.holdings[symbol]; holds {
		return wallet.refuse(symbol, actionEnter, reasonHolding)
	}

	if wallet.pending(symbol) {
		return wallet.refuse(symbol, actionEnter, reasonPending)
	}
	pair, found := wallet.pairs[symbol]

	if !found {
		return wallet.refuse(symbol, actionEnter, reasonUnpriced)
	}

	if pair.Quote != standing.currency {
		return wallet.refuse(symbol, actionEnter, reasonCurrency)
	}
	budget, err := times(wallet.available(), standing.fraction)

	if err != nil {
		return nil, err
	}

	if budget.Sign() <= 0 {
		return wallet.refuse(symbol, actionEnter, reasonBelowMinimum)
	}
	fee, err := wallet.rate(pair)

	if err != nil {
		return nil, err
	}
	scale := int64(pair.CostDecimals)
	unit := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(scale), nil))
	increment, err := exactly(unit, scale)

	if err != nil {
		return nil, err
	}
	notional, err := floorTo(budget, plus(one(), fee), increment)

	if err != nil {
		return nil, err
	}
	return wallet.place(symbol, sideBuy, notional, budget), nil
}

/* exit sells the whole holding in symbol. */
func (wallet *account) exit(symbol string) (*order, error) {
	position, holds := wallet.holdings[symbol]

	if !holds {
		return wallet.refuse(symbol, actionExit, reasonFlat)
	}

	if wallet.pending(symbol) {
		return wallet.refuse(symbol, actionExit, reasonPending)
	}
	return wallet.place(symbol, sideSell, position.quantity, zero()), nil
}

func (wallet *account) place(symbol, side string, size, reserved *decimal.Decimal) *order {
	wallet.issued++
	wallet.orders[wallet.issued] = outstanding{symbol: symbol, side: side, reserved: reserved}
	return &order{id: wallet.issued, symbol: symbol, side: side, amount: size}
}

/* settle applies what the venue executed for one of this account's orders. */
func (wallet *account) settle(fill execution) error {
	placed, found := wallet.orders[fill.order]

	if !found {
		return errnie.Error(errnie.Err(errnie.Validation, "account: fill for an order this account did not place", nil))
	}
	delete(wallet.orders, fill.order)
	action := actionEnter

	if placed.side == sideSell {
		action = actionExit
	}

	if fill.reason != "" {
		_, err := wallet.refuse(placed.symbol, action, fill.reason)
		return err
	}

	if err := wallet.advance(fill.time); err != nil {
		return err
	}
	pair, found := wallet.pairs[placed.symbol]

	if !found {
		return errnie.Error(errnie.Err(errnie.Validation, "account: fill on a pair missing from the fee schedule", nil))
	}
	fraction, err := wallet.rate(pair)

	if err != nil {
		return err
	}
	fee, err := times(fill.cost, fraction)

	if err != nil {
		return err
	}
	wallet.volume = append(wallet.volume, traded{at: wallet.now, volume: fill.cost})

	if placed.side == sideBuy {
		outlay := plus(fill.cost, fee)
		wallet.cash = minus(wallet.cash, outlay)
		wallet.holdings[placed.symbol] = &holding{
			quantity: fill.quantity, basis: outlay, spent: outlay, proceeds: zero(), opened: fill.time,
		}
		return nil
	}
	return wallet.sold(placed.symbol, fill.quantity, minus(fill.cost, fee), fill.time)
}

/* sold allocates basis to what was sold and keeps the rest by subtraction. */
func (wallet *account) sold(symbol string, quantity, net *decimal.Decimal, moment string) error {
	position, found := wallet.holdings[symbol]

	if !found {
		return errnie.Error(errnie.Err(errnie.Validation, "account: sale of a symbol this account does not hold", nil))
	}

	if quantity.Cmp(position.quantity) > 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "account: sold more than was held", nil))
	}
	allocated, err := allocate(position.basis, quantity, position.quantity)

	if err != nil {
		return err
	}
	wallet.cash = plus(wallet.cash, net)
	position.proceeds = plus(position.proceeds, net)
	position.basis = minus(position.basis, allocated)
	position.quantity = minus(position.quantity, quantity)

	if position.quantity.Sign() > 0 {
		return nil
	}
	delete(wallet.holdings, symbol)
	pnl := minus(position.proceeds, position.spent)
	ratio, _ := new(big.Rat).Quo(pnl.Rat(), position.spent.Rat()).Float64()
	wallet.outcomes = append(wallet.outcomes, outcome{closed: &closure{
		symbol: symbol, opened: position.opened, closed: moment,
		basis: position.spent, proceeds: position.proceeds, pnl: pnl, ratio: ratio,
	}})
	return nil
}

/* report hands out the oldest outcome not yet reported. */
func (wallet *account) report() (outcome, bool) {
	if len(wallet.outcomes) == 0 {
		return outcome{}, false
	}
	reported := wallet.outcomes[0]
	wallet.outcomes = wallet.outcomes[1:]
	return reported, true
}
