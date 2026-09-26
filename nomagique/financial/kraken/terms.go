package kraken

import (
	"context"
	venue "github.com/krakenfx/api-go/v2/pkg/kraken"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

/* TermsServer owns authoritative instrument normalization and account fees. */
type TermsServer struct {
	api        *spot.REST
	normalizer *spot.Normalizer
	loaded     bool
}

/* NewTerms creates an idle venue node; credentials arrive through graph inputs. */
func NewTerms() *TermsServer {
	api := spot.NewREST()
	// SDK counters append three decimal digits. Microseconds fit the protocol's
	// UInt64 nonce and avoid the SDK's recursive locked rollover at 1,000/s.
	counter := venue.NewEpochCounter()
	counter.Granularity = time.Microsecond
	api.Nonce = counter.Get
	return &TermsServer{api: api, normalizer: spot.NewNormalizer()}
}

/* Write configures authentication without making an external request. */
func (server *TermsServer) Write(ctx context.Context, call Terms_write) error {
	public, err := call.Args().PublicKey()

	if err != nil {
		return errnie.Error(err)
	}
	private, err := call.Args().PrivateKey()

	if err != nil {
		return errnie.Error(err)
	}

	if len(public) == 0 || len(private) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken terms: trading credentials are required", nil))
	}
	server.api.PublicKey, server.api.PrivateKey = string(public), string(private)
	return nil
}

/* Done completes configuration; the graph passes this capability to its consumer. */
func (server *TermsServer) Done(ctx context.Context, call Terms_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	result.SetReady(server.api.PublicKey != "" && server.api.PrivateKey != "")
	return nil
}

/* Quote reads the account's actual fee and the SDK-normalized venue increments. */
func (server *TermsServer) Quote(ctx context.Context, call Terms_quote) error {
	symbol, err := call.Args().Symbol()

	if err != nil {
		return errnie.Error(err)
	}

	if symbol == "" || server.api.PublicKey == "" || server.api.PrivateKey == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken terms: symbol and configured credentials are required", nil))
	}

	if !server.loaded {
		if err := server.normalizer.Use(server.api); err != nil {
			return errnie.Error(err)
		}
		server.loaded = true
	}
	pair, err := server.normalizer.PairInfo(symbol)

	if err != nil {
		return errnie.Error(err)
	}

	if pair.OrderMinimum == nil || pair.CostMinimum == nil || pair.OrderMinimum.Sign() <= 0 || pair.CostMinimum.Sign() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken terms: venue minimum quantity or cost is missing", nil))
	}
	minimum, err := server.normalizer.FormatSize(symbol, pair.OrderMinimum.SetRounding(core.FloorDecimal))

	if err != nil {
		return errnie.Error(err)
	}
	response, err := spot.Call[struct {
		Fees map[string]struct {
			Fee *decimal.Decimal `json:"fee"`
		} `json:"fees"`
	}](server.api, spot.RequestOptions{
		Auth: true, Method: "POST", Path: "/0/private/TradeVolume",
		Body: struct {
			Pair string `json:"pair"`
		}{pair.AltName},
	})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "kraken terms: account fee lookup", err))
	}
	var fee *decimal.Decimal
	for name, record := range response.Result.Fees {
		if server.normalizer.Name(name) == server.normalizer.Name(symbol) {
			fee = record.Fee
		}
	}

	if fee == nil || fee.Sign() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken terms: venue did not return this market's taker fee", nil))
	}
	// The protocol expresses fees in percent; dividing by 100 is a unit conversion.
	fee = fee.SetRounding(core.FloorDecimal).SetScale(fee.GetScale() + 2).Div(decimal.NewFromInt64(100))
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	terms, err := result.NewTerms()

	if err != nil {
		return errnie.Error(err)
	}
	terms.SetCostPlaces(uint32(pair.CostDecimals))
	for _, err := range []error{
		terms.SetSymbol(server.normalizer.Name(symbol)), terms.SetMinimumQuantity(minimum.String()),
		terms.SetMinimumCost(pair.CostMinimum.String()), terms.SetQuantityIncrement(minimum.GetSmallestIncrement().String()),
		terms.SetTakerFee(fee.String()),
	} {
		if err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}

/* Normalize delegates quantity precision and protocol pair naming to Kraken's SDK. */
func (server *TermsServer) Normalize(ctx context.Context, call Terms_normalize) error {
	symbol, err := call.Args().Symbol()

	if err != nil {
		return errnie.Error(err)
	}
	quantityText, err := call.Args().Quantity()

	if err != nil {
		return errnie.Error(err)
	}
	quantity, err := core.ReadDecimal([]byte(quantityText), "kraken order quantity")

	if err != nil {
		return err
	}

	if quantity.Sign() <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken terms: positive order quantity is required", nil))
	}

	if !server.loaded {
		if err := server.normalizer.Use(server.api); err != nil {
			return errnie.Error(err)
		}
		server.loaded = true
	}
	pair, err := server.normalizer.PairInfo(symbol)

	if err != nil {
		return errnie.Error(err)
	}
	quantity, err = server.normalizer.FormatSize(symbol, quantity.SetRounding(core.FloorDecimal))

	if err != nil {
		return errnie.Error(err)
	}

	if pair.OrderMinimum == nil || quantity.Cmp(pair.OrderMinimum) < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken terms: order quantity is below venue minimum", nil))
	}
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}

	if err := result.SetSymbol(pair.AltName); err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetQuantity(quantity.String()))
}

/* Nonce is the single SDK counter shared by authenticated graph consumers. */
func (server *TermsServer) Nonce(ctx context.Context, call Terms_nonce) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetValue(server.api.Nonce()))
}

/*
	Balance reads cash available for spot trading, excluding borrowed credit.

Kraken BalanceEx defines trade holds separately from the cash balance:
https://docs.kraken.com/api-reference/account-data/get-extended-balance
*/
func (server *TermsServer) Balance(ctx context.Context, call Terms_balance) error {
	symbol, err := call.Args().Symbol()
	if err != nil {
		return errnie.Error(err)
	}
	if !server.loaded {
		if err := server.normalizer.Use(server.api); err != nil {
			return errnie.Error(err)
		}
		server.loaded = true
	}
	pair, err := server.normalizer.PairInfo(symbol)
	if err != nil {
		return errnie.Error(err)
	}
	response, err := spot.Call[map[string]struct {
		Balance *decimal.Decimal `json:"balance"`
		Held    *decimal.Decimal `json:"hold_trade"`
	}](server.api, spot.RequestOptions{Auth: true, Method: "POST", Path: "/0/private/BalanceEx"})
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "kraken terms: available cash", err))
	}
	var available *decimal.Decimal
	for asset, balance := range response.Result {
		if server.normalizer.Name(asset) != server.normalizer.Name(pair.Quote) {
			continue
		}
		if balance.Balance == nil || balance.Held == nil {
			return errnie.Error(errnie.Err(errnie.Validation, "kraken terms: balance or trade hold missing", nil))
		}
		available = balance.Balance.SetScale(max(balance.Balance.GetScale(), balance.Held.GetScale())).Sub(balance.Held)
	}
	if available == nil || available.Sign() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken terms: available quote cash is absent or negative", nil))
	}
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetAvailable(available.String()))
}
