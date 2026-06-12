package response

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
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

/*
Balances simulates the Kraken balances channel on the shared raw bus.

mu guards model/costBasis/realized: fills arrive both from the paper matching
tick (resting triggers, pending takers) and from the quote cache's trade-listener
goroutine (maker queue fills), so wallet state is mutated from two goroutines.
*/
type Balances struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q[any]
	isActive      atomic.Bool
	observers     []types.Socket
	quoteCurrency string
	mu            sync.Mutex
	model         user.Balances
	catalog       *PairCatalog
	realized      *big.Rat // running net realized P&L over the session
	holdings      map[string]*big.Rat
	costBasis     map[string]*big.Rat // fee-inclusive average cost per base asset
	symbols       map[string]string
	marks         map[string]float64
	expectedExit  map[string]float64
	unrealized    map[string]float64
	exitFeeRates  map[string]float64
}

func NewBalances(
	ctx context.Context, pool *qpool.Q[any], catalog *PairCatalog,
) *Balances {
	ctx, cancel := context.WithCancel(ctx)

	quote := strings.ToUpper(viper.GetString("market.quote_currency"))

	return &Balances{
		ctx:           ctx,
		cancel:        cancel,
		err:           nil,
		pool:          pool,
		observers:     make([]types.Socket, 0),
		quoteCurrency: quote,
		model: user.Balances{
			Asset: []user.Balance{
				{
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
				},
			},
		},
		catalog:      catalog,
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
		out = &types.SocketMessage{
			Channel: "balances",
			Success: &[]bool{true}[0],
		}
	default:
		return nil
	}

	balances.mu.Lock()
	data, err := sonic.Marshal(balances.snapshotLocked())
	balances.mu.Unlock()

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

	balances.mu.Lock()
	defer balances.mu.Unlock()

	notional := params.OrderQty * fillPrice

	feeRate, feeErr := balances.catalog.FeeRate(params.Symbol, params.OrderType)

	if feeErr != nil {
		return user.Execution{}, feeErr
	}

	fee := notional * feeRate
	liquidity := "t"

	if params.OrderType == trading.Limit {
		liquidity = "m"
	}

	if recordErr := balances.catalog.RecordFill(params.Symbol, notional); recordErr != nil {
		return user.Execution{}, recordErr
	}

	quantity := new(big.Rat).SetFloat64(params.OrderQty)
	held := balances.holdings[base]

	if held == nil {
		held = new(big.Rat)
		balances.holdings[base] = held
	}

	basis := balances.costBasis[base]

	if basis == nil {
		basis = new(big.Rat)
		balances.costBasis[base] = basis
	}

	switch params.Side {
	case trading.Buy:
		cost := notional + fee
		cash := quoteBalance(&balances.model, balances.quoteCurrency)

		if cash < cost {
			return user.Execution{}, ErrInsufficientFunds
		}

		setQuoteBalance(&balances.model, balances.quoteCurrency, cash-cost)

		// Fee-inclusive average cost: (held*basis + cost) / (held + qty).
		total := new(big.Rat).Mul(held, basis)
		total.Add(total, new(big.Rat).SetFloat64(cost))
		held.Add(held, quantity)
		basis.Quo(total, held)
		balances.symbols[base] = params.Symbol
	case trading.Sell:
		if held.Cmp(quantity) < 0 {
			return user.Execution{}, ErrInsufficientHoldings
		}

		proceeds := notional - fee
		setQuoteBalance(
			&balances.model,
			balances.quoteCurrency,
			quoteBalance(&balances.model, balances.quoteCurrency)+proceeds,
		)

		held.Sub(held, quantity)

		gain := new(big.Rat).SetFloat64(proceeds)
		gain.Sub(gain, new(big.Rat).Mul(quantity, basis))
		balances.realized.Add(balances.realized, gain)

		if held.Sign() == 0 {
			basis.SetInt64(0)
			delete(balances.symbols, base)
			delete(balances.expectedExit, base)
			delete(balances.unrealized, base)
			delete(balances.exitFeeRates, base)
			delete(balances.marks, params.Symbol)
		}
	default:
		return user.Execution{}, ErrInvalidFillParams
	}

	heldFloat, _ := held.Float64()
	setAssetBalance(&balances.model, base, heldFloat)

	execution := user.Execution{
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
	}

	return execution, nil
}

func (balances *Balances) UpdateTicker(ticker *market.TickerUpdate) bool {
	if balances == nil || balances.catalog == nil || ticker == nil || ticker.Bid <= 0 {
		return false
	}

	base := baseAsset(ticker.Symbol)

	if base == "" {
		return false
	}

	balances.mu.Lock()
	quantity := ratFloat(balances.holdings[base])
	entry := ratFloat(balances.costBasis[base])
	balances.mu.Unlock()

	if quantity <= 0 || entry <= 0 {
		return false
	}

	feeRate, feeErr := balances.catalog.FeeRate(ticker.Symbol, trading.Market)

	if feeErr != nil {
		return false
	}

	balances.mu.Lock()
	defer balances.mu.Unlock()

	quantity = ratFloat(balances.holdings[base])
	entry = ratFloat(balances.costBasis[base])

	if quantity <= 0 || entry <= 0 {
		return false
	}

	expectedExit := quantity * ticker.Bid * (1 - feeRate)
	unrealized := expectedExit - (quantity * entry)

	if balances.marks[ticker.Symbol] == ticker.Bid &&
		balances.expectedExit[base] == expectedExit &&
		balances.unrealized[base] == unrealized &&
		balances.exitFeeRates[base] == feeRate {
		return false
	}

	balances.symbols[base] = ticker.Symbol
	balances.marks[ticker.Symbol] = ticker.Bid
	balances.expectedExit[base] = expectedExit
	balances.unrealized[base] = unrealized
	balances.exitFeeRates[base] = feeRate

	return true
}

func (balances *Balances) Wallet() user.Balances {
	balances.mu.Lock()
	defer balances.mu.Unlock()

	return balances.snapshotLocked()
}

func (balances *Balances) ModelJSON() ([]byte, error) {
	balances.mu.Lock()
	defer balances.mu.Unlock()

	return sonic.Marshal(balances.snapshotLocked())
}

func (balances *Balances) snapshotLocked() user.Balances {
	wallet := user.Balances{
		Asset: make([]user.Balance, len(balances.model.Asset)),
	}
	copy(wallet.Asset, balances.model.Asset)

	for index := range wallet.Asset {
		wallet.Asset[index].Wallets = copiedWallets(
			balances.model.Asset[index].Wallets,
		)
	}

	balances.enrichSnapshotLocked(&wallet)

	return wallet
}

func (balances *Balances) enrichSnapshotLocked(wallet *user.Balances) {
	wallet.Currency = balances.quoteCurrency
	wallet.Balance = quoteBalance(&balances.model, balances.quoteCurrency)
	wallet.Inventory = make(map[string]float64, len(balances.holdings))
	wallet.AvgEntry = make(map[string]float64, len(balances.costBasis))
	wallet.Marks = copyFloatMap(balances.marks)
	wallet.Expected = copyFloatMap(balances.expectedExit)
	wallet.Unrealized = copyFloatMap(balances.unrealized)
	wallet.ExitFeeRate = copyFloatMap(balances.exitFeeRates)
	wallet.Realized = ratFloat(balances.realized)

	for base, held := range balances.holdings {
		quantity := ratFloat(held)

		if quantity <= 0 {
			continue
		}

		wallet.Inventory[base] = quantity

		if basis := balances.costBasis[base]; basis != nil {
			wallet.AvgEntry[base] = ratFloat(basis)
		}
	}
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}

	copied := make(map[string]float64, len(values))

	for key, value := range values {
		copied[key] = value
	}

	return copied
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
