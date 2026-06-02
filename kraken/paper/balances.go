package paper

import (
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

/*
Balances simulates the Kraken balances channel on the shared raw bus.
*/
type Balances struct {
	socket     *WebSocket
	identifier *Identifier
	catalog    *PairCatalog
	mu         sync.Mutex
	quote      string
	assets     map[string]float64
	sequence   int
}

func NewBalances(
	socket *WebSocket,
	identifier *Identifier,
	catalog *PairCatalog,
) *Balances {
	quote := viper.GetViper().GetString("market.quote_currency")

	if quote == "" {
		quote = "EUR"
	}

	assets := make(map[string]float64)
	seed := viper.GetViper().GetFloat64("trading.paper.wallet_eur")

	if seed > 0 {
		assets[quote] = seed
	}

	return &Balances{
		socket:     socket,
		identifier: identifier,
		catalog:    catalog,
		quote:      quote,
		assets:     assets,
	}
}

func (balances *Balances) Send(message *qpool.QValue[any]) public.SocketMessage {
	if _, ok := message.Value.(user.SubscribeFrame); ok {
		return balances.snapshot()
	}

	frame, ok := message.Value.(map[string]any)

	if !ok {
		return public.SocketMessage{}
	}

	switch frame["method"] {
	case "subscribe":
		if rows, ok := frame["data"].([]user.Balance); ok {
			return balances.message(user.BalanceUpdate, rows)
		}

		return balances.snapshot()
	}

	return public.SocketMessage{}
}

func (balances *Balances) ApplyFill(
	symbol, side string,
	qty, price, fee float64,
	refID string,
) {
	base := balances.catalog.baseAsset(symbol)
	quote := balances.catalog.quoteAsset(symbol)
	cost := qty * price
	now := time.Now().UTC().Format(time.RFC3339Nano)

	balances.mu.Lock()

	var rows []user.Balance

	if side == "buy" {
		balances.assets[base] += qty
		balances.assets[quote] -= cost + fee

		rows = []user.Balance{
			balances.tradeRow(base, qty, balances.assets[base], 0, refID, now),
			balances.tradeRow(quote, -(cost + fee), balances.assets[quote], fee, refID, now),
		}
	} else {
		balances.assets[base] -= qty
		baseBalance := balances.assets[base]

		if baseBalance <= 0 {
			delete(balances.assets, base)
			baseBalance = 0
		}

		balances.assets[quote] += cost - fee

		rows = []user.Balance{
			balances.tradeRow(base, -qty, baseBalance, 0, refID, now),
			balances.tradeRow(quote, cost-fee, balances.assets[quote], fee, refID, now),
		}
	}

	balances.mu.Unlock()

	channel := balances.socket.broadcasts["kraken:private"]

	for _, row := range rows {
		channel.Send(&qpool.QValue[any]{
			Type: public.BalancesChannel,
			Value: map[string]any{
				"method": "subscribe",
				"data":   []user.Balance{row},
			},
		})
	}
}

func (balances *Balances) snapshot() public.SocketMessage {
	balances.mu.Lock()
	rows := make([]user.Balance, 0, len(balances.assets))

	for asset, amount := range balances.assets {
		if amount <= 0 {
			continue
		}

		rows = append(rows, balances.snapshotRow(asset, amount))
	}

	balances.mu.Unlock()

	return balances.message(user.BalanceSnapshot, rows)
}

func (balances *Balances) message(kind string, rows []user.Balance) public.SocketMessage {
	if len(rows) == 0 {
		return public.SocketMessage{}
	}

	balances.mu.Lock()
	balances.sequence++
	balances.mu.Unlock()

	data, err := sonic.Marshal(rows)

	if err != nil {
		return public.SocketMessage{}
	}

	return public.SocketMessage{
		Channel: public.BalancesChannel,
		Type:    kind,
		Data:    data,
	}
}

func (balances *Balances) snapshotRow(asset string, amount float64) user.Balance {
	return user.Balance{
		Asset:      asset,
		AssetClass: "currency",
		Balance:    amount,
		Wallets: []user.BalanceWallet{{
			Balance: amount,
			Type:    "spot",
			ID:      "main",
		}},
	}
}

func (balances *Balances) tradeRow(
	asset string,
	amount, balance, fee float64,
	refID, timestamp string,
) user.Balance {
	return user.Balance{
		Asset:      asset,
		AssetClass: "currency",
		Amount:     amount,
		Balance:    balance,
		Fee:        fee,
		LedgerID:   strings.ToUpper(balances.identifier.LedgerID()),
		RefID:      refID,
		Timestamp:  timestamp,
		Type:       "trade",
		Category:   "trade",
		WalletType: "spot",
		WalletID:   "main",
	}
}
