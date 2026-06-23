package trader

import (
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
wireUIBalances republishes Kraken balances channel snapshots for the dashboard.
Paper and live private websockets both publish identical balances artifacts.
*/
func (crypto *Crypto) wireUIBalances() {
	if crypto == nil || crypto.pool == nil || crypto.uiBroadcast == nil {
		return
	}

	crypto.pool.Subscribe("balances", func(artifact *datura.Artifact) error {
		if artifact == nil {
			return nil
		}

		payload := artifact.DecryptPayload()

		if len(payload) == 0 {
			return nil
		}

		var wire map[string]any

		if err := sonic.Unmarshal(payload, &wire); err != nil {
			return errnie.Err(
				errnie.Validation,
				"trader: balances bridge decode failed",
				err,
			)
		}

		quoteCurrency := strings.ToUpper(viper.GetString("market.quote_currency"))
		wire["type"] = "wallet"
		wire["Currency"] = quoteCurrency
		wire["assets"] = map[string]any{
			"asset": wire["asset"],
		}

		if rows, ok := wire["asset"].([]any); ok {
			for _, row := range rows {
				entry, rowOK := row.(map[string]any)

				if !rowOK {
					continue
				}

				asset, _ := entry["asset"].(string)
				balance, _ := entry["balance"].(float64)

				if asset == quoteCurrency ||
					asset == "Z"+quoteCurrency ||
					asset == "ZUSD" && quoteCurrency == "USD" {
					wire["Balance"] = balance

					break
				}
			}
		}

		out := datura.Acquire("trader-wallet", datura.APPJSON)
		out.WithDestination("ui")
		out.WithRole("wallet")

		if err := out.From(wire); err != nil {
			return errnie.Err(
				errnie.Validation,
				"trader: balances bridge marshal failed",
				err,
			)
		}

		if err := crypto.uiBroadcast.Send(out); err != nil {
			return errnie.Err(
				errnie.IO,
				"trader: balances bridge publish failed",
				err,
			)
		}

		return nil
	})
}
