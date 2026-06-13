package response

import (
	"context"
	"errors"
	"math"
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

// ErrInsufficientFunds rejects a fill that the wallet cannot fund in the spent currency.
var ErrInsufficientFunds = errors.New("paper balances: insufficient funds")

// ErrInsufficientHoldings rejects a sell of more base asset than the wallet holds.
var ErrInsufficientHoldings = errors.New("paper balances: insufficient holdings")

// ErrInvalidFillParams rejects fills with non-positive quantity or price.
var ErrInvalidFillParams = errors.New("paper balances: invalid fill params")

type walletState struct {
	model        user.Balances
	realized     *big.Rat
	holdings     map[string]*big.Rat
	costBasis    map[string]*big.Rat
	symbols      map[string]string
	marks        map[string]float64
	expectedExit map[string]float64
	unrealized   map[string]float64
	exitFeeRates map[string]float64
}

/*
Balances simulates the Kraken balances channel on the shared raw bus.

Wallet state is immutable per snapshot and swapped with atomic.Pointer so
concurrent fill and mark paths never use mutexes or channels.
*/
type Balances struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q[any]
	isActive      atomic.Bool
	observers     []types.Socket
	quoteCurrency string
	state         atomic.Pointer[walletState]
	catalog       *PairCatalog
}

func NewBalances(
	ctx context.Context, pool *qpool.Q[any], catalog *PairCatalog,
) *Balances {
	ctx, cancel := context.WithCancel(ctx)

	quote := strings.ToUpper(viper.GetString("market.quote_currency"))
	balances := &Balances{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		observers:     make([]types.Socket, 0),
		quoteCurrency: quote,
		catalog:       catalog,
	}

	initial := newWalletState(user.Balances{
		Asset: []user.Balance{{
			Asset:      viper.GetString("market.quote_currency"),
			AssetClass: "currency",
			Balance: viper.GetFloat64(
				"trading.paper.wallet." + strings.ToLower(quote),
			),
			Wallets: []user.BalanceWallet{{
				Balance: viper.GetFloat64(
					"trading.paper.wallet." + strings.ToLower(quote),
				),
				Type: "spot",
				ID:   "main",
			}},
		}},
	})
	balances.state.Store(initial)

	return balances
}

func newWalletState(model user.Balances) *walletState {
	return &walletState{
		model:        cloneModelBalances(model),
		realized:     new(big.Rat),
		holdings:     make(map[string]*big.Rat),
		costBasis:    make(map[string]*big.Rat),
		symbols:      make(map[string]string),
		marks:        make(map[string]float64),
		expectedExit: make(map[string]float64),
		unrealized:   make(map[string]float64),
		exitFeeRates: make(map[string]float64),
	}
}

func cloneWalletState(state *walletState) *walletState {
	if state == nil {
		return newWalletState(user.Balances{})
	}

	return &walletState{
		model:        cloneModelBalances(state.model),
		realized:     cloneRat(state.realized),
		holdings:     cloneRatMap(state.holdings),
		costBasis:    cloneRatMap(state.costBasis),
		symbols:      stringMapsCopy(state.symbols),
		marks:        mapsCopy(state.marks),
		expectedExit: mapsCopy(state.expectedExit),
		unrealized:   mapsCopy(state.unrealized),
		exitFeeRates: mapsCopy(state.exitFeeRates),
	}
}

func (balances *Balances) swapState(mutate func(*walletState) error) error {
	for {
		current := balances.state.Load()
		next := cloneWalletState(current)

		if err := mutate(next); err != nil {
			return err
		}

		if balances.state.CompareAndSwap(current, next) {
			return nil
		}
	}
}

func (balances *Balances) Send(message *qpool.QValue[any]) *types.SocketMessage {
	frame, ok := message.Value.(types.KrakenMessage)

	if !ok {
		return nil
	}

	var out *types.SocketMessage

	switch frame.Method {
	case "subscribe":
		balances.isActive.Store(true)
	case "unsubscribe":
		balances.isActive.Store(false)
	default:
		return nil
	}

	data, err := sonic.Marshal(balances.Wallet())

	if err != nil {
		return nil
	}

	out = &types.SocketMessage{
		Channel: "balances",
		Success: &[]bool{true}[0],
		Data:    data,
	}

	for _, observer := range balances.observers {
		observer.Send(&qpool.QValue[any]{Value: out})
	}

	return out
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers = append(balances.observers, socket)
	}
}

/*
ApplyFill mutates the paper wallet for a taker fill and returns the execution row.
*/
func (balances *Balances) ApplyFill(
	params trading.AddParams,
	fillPrice float64,
) (user.Execution, error) {
	if params.OrderQty <= 0 || fillPrice <= 0 {
		return user.Execution{}, ErrInvalidFillParams
	}

	base := baseAsset(params.Symbol)

	if base == "" {
		return user.Execution{}, ErrInvalidFillParams
	}

	var execution user.Execution

	swapErr := balances.swapState(func(state *walletState) error {
		var applyErr error
		execution, applyErr = applyFillState(
			state,
			balances.catalog,
			balances.quoteCurrency,
			params,
			fillPrice,
			base,
		)

		return applyErr
	})

	if swapErr != nil {
		return user.Execution{}, swapErr
	}

	return execution, nil
}

func applyFillState(
	state *walletState,
	catalog *PairCatalog,
	quoteCurrency string,
	params trading.AddParams,
	fillPrice float64,
	base string,
) (user.Execution, error) {
	held := state.holdings[base]

	if params.Side == trading.Sell && held != nil {
		quantity := new(big.Rat).SetFloat64(params.OrderQty)
		diff := new(big.Rat).Sub(quantity, held)
		diffFloat, _ := diff.Float64()

		if held.Cmp(quantity) < 0 && math.Abs(diffFloat) < 1e-5 {
			params.OrderQty, _ = held.Float64()
		}
	}

	notional := params.OrderQty * fillPrice

	feeRate, feeErr := catalog.FeeRate(params.Symbol, params.OrderType)

	if feeErr != nil {
		return user.Execution{}, feeErr
	}

	fee := notional * feeRate
	liquidity := "t"

	if params.OrderType == trading.Limit {
		liquidity = "m"
	}

	if recordErr := catalog.RecordFill(params.Symbol, notional); recordErr != nil {
		return user.Execution{}, recordErr
	}

	quantity := new(big.Rat).SetFloat64(params.OrderQty)
	held = state.holdings[base]

	if held == nil {
		held = new(big.Rat)
		state.holdings[base] = held
	}

	basis := state.costBasis[base]

	if basis == nil {
		basis = new(big.Rat)
		state.costBasis[base] = basis
	}

	switch params.Side {
	case trading.Buy:
		cost := notional + fee
		quote := resolveQuoteCurrency(quoteCurrency, state)
		cash := quoteBalance(&state.model, quote)

		if cash < cost {
			return user.Execution{}, ErrInsufficientFunds
		}

		setQuoteBalance(&state.model, quote, cash-cost)

		total := new(big.Rat).Mul(held, basis)
		total.Add(total, new(big.Rat).SetFloat64(cost))
		held.Add(held, quantity)
		basis.Quo(total, held)
		state.symbols[base] = params.Symbol
	case trading.Sell:
		if held.Cmp(quantity) < 0 {
			return user.Execution{}, ErrInsufficientHoldings
		}

		proceeds := notional - fee
		quote := resolveQuoteCurrency(quoteCurrency, state)
		setQuoteBalance(
			&state.model,
			quote,
			quoteBalance(&state.model, quote)+proceeds,
		)

		held.Sub(held, quantity)

		gain := new(big.Rat).SetFloat64(proceeds)
		gain.Sub(gain, new(big.Rat).Mul(quantity, basis))
		state.realized.Add(state.realized, gain)

		if held.Sign() == 0 {
			basis.SetInt64(0)
			delete(state.symbols, base)
			delete(state.expectedExit, base)
			delete(state.unrealized, base)
			delete(state.exitFeeRates, base)
			delete(state.marks, params.Symbol)
		}
	default:
		return user.Execution{}, ErrInvalidFillParams
	}

	heldFloat, _ := held.Float64()
	setAssetBalance(&state.model, base, heldFloat)

	if held.Sign() > 0 {
		entryFloat, _ := basis.Float64()
		expectedExit := heldFloat * fillPrice * (1 - feeRate)

		state.marks[params.Symbol] = fillPrice
		state.expectedExit[base] = expectedExit
		state.unrealized[base] = expectedExit - (heldFloat * entryFloat)
		state.exitFeeRates[base] = feeRate
	}

	return user.Execution{
		OrderID:      params.ClOrdID,
		ClOrdID:      params.ClOrdID,
		Symbol:       params.Symbol,
		Side:         string(params.Side),
		OrderType:    string(params.OrderType),
		OrderQty:     params.OrderQty,
		LimitPrice:   params.LimitPrice,
		OrderStatus:  "filled",
		ExecType:     "trade",
		ExecID:       params.ClOrdID + "-fill",
		LastQty:      params.OrderQty,
		LastPrice:    fillPrice,
		AvgPrice:     fillPrice,
		CumQty:       params.OrderQty,
		CumCost:      notional,
		Cost:         notional,
		LiquidityInd: liquidity,
		FeeUsdEquiv:  fee,
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func resolveQuoteCurrency(quoteCurrency string, state *walletState) string {
	if quoteCurrency != "" {
		return quoteCurrency
	}

	return quoteFromModel(state.model)
}

func quoteFromModel(model user.Balances) string {
	for _, asset := range model.Asset {
		name := strings.ToUpper(strings.TrimSpace(asset.Asset))

		if name != "" {
			return strings.TrimPrefix(name, "Z")
		}
	}

	return "USD"
}

func (balances *Balances) UpdateTicker(ticker *market.TickerUpdate) bool {
	if balances == nil || balances.catalog == nil || ticker == nil || ticker.Bid <= 0 {
		return false
	}

	base := baseAsset(ticker.Symbol)

	if base == "" {
		return false
	}

	changed := false

	swapErr := balances.swapState(func(state *walletState) error {
		quantity := ratFloat(state.holdings[base])
		entry := ratFloat(state.costBasis[base])

		if quantity <= 0 || entry <= 0 {
			return nil
		}

		feeRate, feeErr := balances.catalog.FeeRate(ticker.Symbol, trading.Market)

		if feeErr != nil {
			return feeErr
		}

		expectedExit := quantity * ticker.Bid * (1 - feeRate)
		unrealized := expectedExit - (quantity * entry)

		if state.marks[ticker.Symbol] == ticker.Bid &&
			state.expectedExit[base] == expectedExit &&
			state.unrealized[base] == unrealized &&
			state.exitFeeRates[base] == feeRate {
			return nil
		}

		state.symbols[base] = ticker.Symbol
		state.marks[ticker.Symbol] = ticker.Bid
		state.expectedExit[base] = expectedExit
		state.unrealized[base] = unrealized
		state.exitFeeRates[base] = feeRate
		changed = true

		return nil
	})

	if swapErr != nil {
		return false
	}

	return changed
}

func (balances *Balances) Wallet() user.Balances {
	state := balances.state.Load()

	if state == nil {
		return user.Balances{}
	}

	return snapshotFromState(state, balances.quoteCurrency)
}

func (balances *Balances) ModelJSON() ([]byte, error) {
	return sonic.Marshal(balances.Wallet())
}

func snapshotFromState(state *walletState, quoteCurrency string) user.Balances {
	wallet := user.Balances{
		Asset: make([]user.Balance, len(state.model.Asset)),
	}
	copy(wallet.Asset, state.model.Asset)

	for index := range wallet.Asset {
		wallet.Asset[index].Wallets = copiedWallets(
			state.model.Asset[index].Wallets,
		)
	}

	enrichSnapshot(&wallet, state, quoteCurrency)

	return wallet
}

func enrichSnapshot(wallet *user.Balances, state *walletState, quoteCurrency string) {
	wallet.Currency = quoteCurrency
	wallet.Balance = quoteBalance(&state.model, quoteCurrency)
	wallet.Inventory = make(map[string]float64, len(state.holdings))
	wallet.AvgEntry = make(map[string]float64, len(state.costBasis))
	wallet.Marks = mapsCopy(state.marks)
	wallet.Expected = mapsCopy(state.expectedExit)
	wallet.Unrealized = mapsCopy(state.unrealized)
	wallet.ExitFeeRate = mapsCopy(state.exitFeeRates)
	wallet.Realized = ratFloat(state.realized)

	for base, held := range state.holdings {
		quantity := ratFloat(held)

		if quantity <= 0 {
			continue
		}

		wallet.Inventory[base] = quantity

		if basis := state.costBasis[base]; basis != nil {
			wallet.AvgEntry[base] = ratFloat(basis)
		}
	}
}

func cloneModelBalances(model user.Balances) user.Balances {
	cloned := model
	cloned.Asset = append([]user.Balance(nil), model.Asset...)

	for index := range cloned.Asset {
		cloned.Asset[index].Wallets = copiedWallets(cloned.Asset[index].Wallets)
	}

	return cloned
}

func cloneRat(value *big.Rat) *big.Rat {
	if value == nil {
		return new(big.Rat)
	}

	return new(big.Rat).Set(value)
}

func cloneRatMap(values map[string]*big.Rat) map[string]*big.Rat {
	cloned := make(map[string]*big.Rat, len(values))

	for key, value := range values {
		cloned[key] = cloneRat(value)
	}

	return cloned
}

func mapsCopy(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return make(map[string]float64)
	}

	cloned := make(map[string]float64, len(values))

	for key, value := range values {
		cloned[key] = value
	}

	return cloned
}

func stringMapsCopy(values map[string]string) map[string]string {
	if len(values) == 0 {
		return make(map[string]string)
	}

	cloned := make(map[string]string, len(values))

	for key, value := range values {
		cloned[key] = value
	}

	return cloned
}

func copiedWallets(wallets []user.BalanceWallet) []user.BalanceWallet {
	if len(wallets) == 0 {
		return nil
	}

	return append([]user.BalanceWallet(nil), wallets...)
}

func ratFloat(value *big.Rat) float64 {
	if value == nil {
		return 0
	}

	floatValue, _ := value.Float64()

	return floatValue
}

func baseAsset(symbol string) string {
	base, _, found := strings.Cut(symbol, "/")

	if !found {
		return ""
	}

	return strings.ToUpper(strings.TrimSpace(base))
}

func setAssetBalance(balances *user.Balances, asset string, amount float64) {
	for index, existing := range balances.Asset {
		if !strings.EqualFold(existing.Asset, asset) {
			continue
		}

		balances.Asset[index].Balance = amount

		if len(balances.Asset[index].Wallets) > 0 {
			balances.Asset[index].Wallets[0].Balance = amount
		}

		return
	}

	balances.Asset = append(balances.Asset, user.Balance{
		Asset:      asset,
		AssetClass: "currency",
		Balance:    amount,
		Wallets: []user.BalanceWallet{{
			Balance: amount,
			Type:    "spot",
			ID:      "main",
		}},
	})
}

func quoteBalance(balances *user.Balances, quote string) float64 {
	for index, asset := range balances.Asset {
		name := strings.ToUpper(asset.Asset)

		if name != quote && name != "Z"+quote {
			continue
		}

		return balances.Asset[index].Balance
	}

	return 0
}

func setQuoteBalance(balances *user.Balances, quote string, amount float64) {
	for index, asset := range balances.Asset {
		name := strings.ToUpper(asset.Asset)

		if name != quote && name != "Z"+quote {
			continue
		}

		balances.Asset[index].Balance = amount

		if len(balances.Asset[index].Wallets) > 0 {
			balances.Asset[index].Wallets[0].Balance = amount
		}

		return
	}
}
