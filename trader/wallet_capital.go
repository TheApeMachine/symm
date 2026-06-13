package trader

import (
	"context"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/user"
)

/*
WalletCapitalProvider tracks live quote balances from exchange wallet snapshots.
*/
type WalletCapitalProvider struct {
	quoteBalances sync.Map
}

func NewWalletCapitalProvider() *WalletCapitalProvider {
	return &WalletCapitalProvider{}
}

func (provider *WalletCapitalProvider) ApplyBalances(balances user.Balances) {
	if provider == nil {
		return
	}

	quote := strings.ToUpper(strings.TrimSpace(balances.Currency))

	if quote == "" {
		quote = "USD"
	}

	if balances.Balance > 0 {
		provider.quoteBalances.Store(quote, balances.Balance)
	}
}

func (provider *WalletCapitalProvider) QuoteBalance(
	ctx context.Context,
	quote string,
) (float64, error) {
	return provider.balanceForQuote(quote)
}

func (provider *WalletCapitalProvider) AvailableQuoteBalance(
	ctx context.Context,
	quote string,
) (float64, error) {
	return provider.balanceForQuote(quote)
}

func (provider *WalletCapitalProvider) balanceForQuote(quote string) (float64, error) {
	if provider == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: wallet capital provider is required",
			nil,
		))
	}

	normalizedQuote := strings.ToUpper(strings.TrimSpace(quote))

	if normalizedQuote == "" {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: quote currency is required",
			nil,
		))
	}

	raw, ok := provider.quoteBalances.Load(normalizedQuote)

	if !ok {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: quote balance is not available yet",
			errnie.Require(map[string]any{
				"quote": normalizedQuote,
			}),
		))
	}

	balance, balanceOK := raw.(float64)

	if !balanceOK || balance <= 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: quote balance must be positive",
			errnie.Require(map[string]any{
				"quote":   normalizedQuote,
				"balance": balance,
			}),
		))
	}

	return balance, nil
}
