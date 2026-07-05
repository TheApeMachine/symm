package broker

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
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
func (book *BalanceBook) Update(frame map[string]any) error {
	if book == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "balance book is nil", nil))
	}

	if len(frame) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "balance frame is empty", nil))
	}

	rows, err := book.rows(frame)
	if err != nil {
		return err
	}

	next := make(map[string]float64, len(rows))
	for _, row := range rows {
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

func (book *BalanceBook) rows(frame map[string]any) ([]map[string]any, error) {
	switch data := frame["data"].(type) {
	case []map[string]any:
		return data, nil
	case []any:
		rows := make([]map[string]any, 0, len(data))
		for _, item := range data {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					"broker: balance data row object required",
					nil,
				))
			}

			rows = append(rows, row)
		}

		return rows, nil
	default:
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: balance data rows required",
			nil,
		))
	}
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

	return 0, errnie.Err(
		errnie.NotFound,
		"broker: balance missing for "+strings.ToUpper(strings.TrimSpace(asset)),
		nil,
	)
}

func (book *BalanceBook) Holdings() (*types.Holdings, error) {
	if book == nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "balance book is nil", nil))
	}

	snapshot := book.snapshot.Load()
	if snapshot == nil || snapshot.funds == nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "broker: balances unavailable", nil))
	}

	holdings := &types.Holdings{
		Rows: make([]types.BalanceRow, 0, len(snapshot.funds)),
	}

	for asset, balance := range snapshot.funds {
		holdings.Rows = append(holdings.Rows, types.BalanceRow{
			Asset:   asset,
			Balance: balance,
		})
	}

	return holdings, nil
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
