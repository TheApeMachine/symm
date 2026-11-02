package broker

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
)

/*
SubscribeExecutions opens the private executions stream. Live sends the
websocket token Kraken requires; paper uses the same subscribe envelope
through the simulator CLI.
*/
func SubscribeExecutions(ctx context.Context, transport Transport, instrument *Instrument) error {
	if transport == nil {
		return nil
	}

	params := map[string]any{
		"channel": "executions",
	}

	if _, live := transport.(*network.WebsocketClient); live {
		if instrument == nil {
			return errnie.Err(
				errnie.Validation,
				"[broker] executions subscribe needs instrument",
				nil,
			)
		}

		token, err := instrument.WebSocketToken(ctx)
		if err != nil {
			return err
		}

		params["token"] = token
		params["snap_orders"] = true
		params["snap_trades"] = false
	}

	buf, err := sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": params,
	})

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[broker] executions subscribe marshal failed",
			err,
		)
	}

	if err := transport.Write(buf); err != nil {
		return errnie.Err(
			errnie.IO,
			"[broker] executions subscribe write failed",
			err,
		)
	}

	return nil
}

// LiveBalances polls the authenticated REST wallet for market.Balance.Follow.
type LiveBalances struct{}

func NewLiveBalances() *LiveBalances {
	return &LiveBalances{}
}

func (live *LiveBalances) Fetch(ctx context.Context) ([]*market.Asset, error) {
	_ = ctx

	client, err := kraken.NewAuthenticatedREST()

	if err != nil {
		return nil, errnie.Error(err)
	}

	resp, err := client.Balances()

	if err != nil {
		return nil, errnie.Error(err)
	}

	if resp == nil || resp.Result == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[live] empty balance response", nil,
		))
	}

	raw, err := sonic.Marshal(resp.Result)

	if err != nil {
		return nil, errnie.Error(err)
	}

	amounts := map[string]string{}

	if err := sonic.Unmarshal(raw, &amounts); err != nil {
		return nil, errnie.Error(err)
	}

	result, err := spot.Call[kraken.TradeBalanceResult](client, spot.RequestOptions{
		Auth:   true,
		Method: "POST",
		Path:   system.Cfg.WebSocket.Endpoints.TradeBalance,
	})

	if err != nil {
		return nil, errnie.Error(err)
	}

	trade := result.Result
	quote := system.Cfg.Market.QuoteCurrency
	assets := make([]*market.Asset, 0, len(amounts))

	for asset, amount := range amounts {
		capital, parseErr := decimal.NewFromString(amount)

		if parseErr != nil {
			continue
		}

		pnl := decimal.Zero
		equity := capital

		if asset == quote || asset == market.Alias(quote) {
			if trade.UnrealizedPnL != nil {
				pnl = shopString(trade.UnrealizedPnL)
			}

			if trade.Equity != nil {
				equity = shopString(trade.Equity)
			}
		}

		assets = append(assets, market.NewAsset(nil, asset, capital, pnl, equity))
	}

	return assets, nil
}

func assetsFromWallet(
	quote string, wallet *kraken.Balance, trade *kraken.TradeBalanceResult,
) []*market.Asset {
	assets := make([]*market.Asset, 0, len(wallet.Data))

	for _, entry := range wallet.Data {
		capital := decimal.Zero

		if entry.Available != nil {
			capital = shopString(entry.Available)
		} else if entry.Balance != nil {
			capital = shopString(entry.Balance)
		}

		pnl := decimal.Zero
		equity := capital

		if entry.Asset == quote || entry.Asset == market.Alias(quote) {
			if trade != nil && trade.UnrealizedPnL != nil {
				pnl = shopString(trade.UnrealizedPnL)
			}

			if trade != nil && trade.Equity != nil {
				equity = shopString(trade.Equity)
			}
		}

		assets = append(assets, market.NewAsset(
			nil, entry.Asset, capital, pnl, equity,
		))
	}

	return assets
}

func shopString(v interface{ String() string }) decimal.Decimal {
	parsed, err := decimal.NewFromString(v.String())

	if err != nil {
		return decimal.Zero
	}

	return parsed
}
