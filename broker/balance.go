package broker

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
BalanceBook is the broker's immutable account snapshot.
It distinguishes a missing asset from an asset whose balance is exactly zero.
*/
type BalanceBook struct {
	snapshot atomic.Pointer[balanceSnapshot]
}

type balanceSnapshot struct {
	funds map[string]float64
}

type balanceFrame struct {
	Data []map[string]any `json:"data"`
}

/*
NewBalanceBook instantiates an empty balance book.
*/
func NewBalanceBook() *BalanceBook {
	book := &BalanceBook{}
	book.snapshot.Store(&balanceSnapshot{funds: map[string]float64{}})

	return book
}

/*
Update replaces the balance book with the latest exchange balance frame.
*/
func (book *BalanceBook) Update(artifact *datura.Artifact) error {
	if book == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "balance book is nil", nil))
	}

	if artifact == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "balance artifact is nil", nil))
	}

	var frame balanceFrame
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &frame); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: decode balance frame",
			err,
		))
	}

	next := make(map[string]float64, len(frame.Data))
	for _, row := range frame.Data {
		asset := strings.ToUpper(strings.TrimSpace(stringValue(row["asset"])))
		if asset == "" {
			continue
		}

		balance, ok := numericValue(row["balance"])
		if !ok {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"broker: balance row missing numeric balance for "+asset,
				nil,
			))
		}

		next[asset] = balance
	}

	if len(next) == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: balance frame contained no assets",
			nil,
		))
	}

	book.snapshot.Store(&balanceSnapshot{funds: next})
	return nil
}

/*
Funds returns the current balance and whether the asset exists in the snapshot.
*/
func (book *BalanceBook) Funds(asset string) (float64, bool) {
	if book == nil {
		return 0, false
	}

	asset = strings.ToUpper(strings.TrimSpace(asset))
	if asset == "" {
		return 0, false
	}

	snapshot := book.snapshot.Load()
	if snapshot == nil || snapshot.funds == nil {
		return 0, false
	}

	funds, ok := snapshot.funds[asset]
	return funds, ok
}

/*
RequireFunds returns a descriptive error when an asset is absent.
*/
func (book *BalanceBook) RequireFunds(asset string) (float64, error) {
	funds, ok := book.Funds(asset)
	if ok {
		return funds, nil
	}

	return 0, errnie.Error(errnie.Err(
		errnie.NotFound,
		"broker: balance missing for "+strings.ToUpper(strings.TrimSpace(asset)),
		nil,
	))
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case jsonNumber:
		parsed, err := strconv.ParseFloat(string(typed), 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

type jsonNumber string

/*
Balance is retained as a narrow compatibility wrapper around BalanceBook.
*/
type Balance struct {
	book *BalanceBook
}

func NewBalance(artifact *datura.Artifact) *Balance {
	book := NewBalanceBook()
	if artifact != nil {
		errnie.Error(book.Update(artifact))
	}

	return &Balance{book: book}
}

func (balance *Balance) Funds(asset string) (float64, error) {
	if balance == nil || balance.book == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.NotFound,
			"broker: balance unavailable",
			nil,
		))
	}

	return balance.book.RequireFunds(asset)
}
