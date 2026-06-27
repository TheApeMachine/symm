package market

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
)

/*
PositionReadings derives one reading per open (non-quote) position from the tree:
the balance frame gives quantity, the executions history gives the entry price of
the last buy/enter fill for that symbol, and the latest ticker frame gives the
mark. Stop state comes from broker stoploss artifacts. Unrealized P&L and percent
change are computed here so nothing downstream invents them. Returns nil when no
balances have been published yet — "no ledger" stays distinct from "flat".
*/
func PositionReadings(tree *dmt.Tree, quoteCurrency string) ([]map[string]any, error) {
	if tree == nil {
		return nil, errnie.Err(errnie.Validation, "positions: nil tree", nil)
	}

	quote := strings.ToUpper(strings.TrimSpace(quoteCurrency))

	balances := latestBalances(tree)

	if balances == nil {
		return nil, nil
	}

	positions := openPositions(balances, quote)
	if len(positions) == 0 {
		return []map[string]any{}, nil
	}

	readings := make([]map[string]any, 0, len(positions))

	for _, position := range positions {
		mark := latestMark(tree, position.symbol)
		if mark <= 0 {
			return nil, errnie.Err(
				errnie.Validation,
				"positions: missing mark for open position "+position.symbol,
				nil,
			)
		}

		entry, entryErr := latestEntryPrice(tree, position.symbol)
		if entryErr != nil {
			return nil, entryErr
		}
		if entry <= 0 {
			return nil, errnie.Err(
				errnie.Validation,
				"positions: missing entry fill for open position "+position.symbol,
				nil,
			)
		}

		reading := map[string]any{
			"symbol":    position.symbol,
			"asset":     position.asset,
			"quote":     quote,
			"quantity":  position.quantity,
			"entry":     entry,
			"mark":      mark,
			"value":     mark * position.quantity,
			"updatedAt": balances.Timestamp(),
		}

		if mark > 0 && entry > 0 {
			reading["unrealizedPnl"] = (mark - entry) * position.quantity
			reading["changePct"] = (mark - entry) / entry * 100
		}

		if stop := latestStop(tree, position.symbol); stop != nil {
			reading["stop"] = datura.Peek[float64](stop, "stop")
			reading["peak"] = datura.Peek[float64](stop, "peak")
			reading["offset"] = datura.Peek[float64](stop, "offset")
			reading["stopSide"] = datura.Peek[string](stop, "side")
		}

		readings = append(readings, reading)
	}

	sort.Slice(readings, func(first, second int) bool {
		return strings.Compare(
			fmt.Sprint(readings[first]["symbol"]),
			fmt.Sprint(readings[second]["symbol"]),
		) < 0
	})

	return readings, nil
}

type openPosition struct {
	asset    string
	symbol   string
	quantity float64
}

func openPositions(balances *datura.Artifact, quote string) []openPosition {
	positions := make([]openPosition, 0)

	for rowIndex := 0; ; rowIndex++ {
		asset := datura.Peek[string](balances, "asset", rowIndex, "asset")

		if asset == "" {
			break
		}

		quantity := datura.Peek[float64](balances, "asset", rowIndex, "balance")

		// The quote currency funds entries rather than being a position, and a
		// dust balance is not an open position.
		if strings.ToUpper(asset) == quote || quantity <= 0 {
			continue
		}

		positions = append(positions, openPosition{
			asset:    asset,
			symbol:   asset + "/" + quote,
			quantity: quantity,
		})
	}

	return positions
}

/*
latestBalances returns the most recent balances frame in the tree, or nil when
none has been published.
*/
func latestBalances(tree *dmt.Tree) *datura.Artifact {
	var (
		latest *datura.Artifact
		stamp  int64
	)

	for artifact := range tree.Seek([]byte("balances/")) {
		if artifact.Timestamp() >= stamp {
			latest = artifact
			stamp = artifact.Timestamp()
		}
	}

	return latest
}

/*
latestEntryPrice walks executions for one held symbol and returns the freshest
opening (buy/enter) fill. Sells are ignored — they close a position, they do not
set an entry basis.
*/
func latestEntryPrice(tree *dmt.Tree, symbol string) (float64, error) {
	type stampedEntry struct {
		price float64
		stamp int64
	}

	var latest stampedEntry
	target := strings.ToUpper(symbol)

	for artifact := range tree.Seek([]byte("executions/")) {
		rows, rowsErr := executionRows(artifact)
		if rowsErr != nil {
			return 0, rowsErr
		}

		for _, row := range rows {
			side := strings.ToLower(row.Side)

			if side != "buy" && side != "enter" {
				continue
			}

			if strings.ToUpper(row.Symbol) != target {
				continue
			}

			price := row.AvgPrice
			if price <= 0 {
				price = row.LastPrice
			}
			if price <= 0 {
				price = row.Price
			}
			if price <= 0 {
				return 0, errnie.Err(
					errnie.Validation,
					"positions: non-positive execution price for "+symbol,
					nil,
				)
			}

			stamp := artifact.Timestamp()

			if latest.stamp > stamp {
				continue
			}

			latest = stampedEntry{price: price, stamp: stamp}
		}
	}

	return latest.price, nil
}

type executionRow struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	AvgPrice  float64 `json:"avg_price"`
	LastPrice float64 `json:"last_price"`
	Price     float64 `json:"price"`
}

func executionRows(artifact *datura.Artifact) ([]executionRow, error) {
	rows := make([]executionRow, 0)

	for rowIndex := 0; ; rowIndex++ {
		symbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")

		if symbol == "" {
			break
		}

		rows = append(rows, executionRow{
			Symbol:    symbol,
			Side:      datura.Peek[string](artifact, "data", rowIndex, "side"),
			AvgPrice:  datura.Peek[float64](artifact, "data", rowIndex, "avg_price"),
			LastPrice: datura.Peek[float64](artifact, "data", rowIndex, "last_price"),
			Price:     datura.Peek[float64](artifact, "data", rowIndex, "price"),
		})
	}

	var payload struct {
		Executions map[string]executionRow `json:"executions"`
	}

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		return nil, errnie.Err(errnie.Validation, "positions: decode executions", err)
	}

	for _, row := range payload.Executions {
		rows = append(rows, row)
	}

	return rows, nil
}

/*
latestMark returns the freshest traded price for one held symbol.
*/
func latestMark(tree *dmt.Tree, symbol string) float64 {
	type stampedMark struct {
		price float64
		stamp int64
	}

	var latest stampedMark
	target := strings.ToUpper(symbol)

	for artifact := range tree.Seek([]byte("ticker/")) {
		for rowIndex := 0; ; rowIndex++ {
			row, err := SymbolFromTicker(artifact, rowIndex)

			if err != nil || row == nil {
				break
			}

			if strings.ToUpper(row.Name) != target {
				continue
			}

			stamp := artifact.Timestamp()

			if latest.stamp > stamp {
				continue
			}

			latest = stampedMark{price: row.Price, stamp: stamp}
		}
	}

	return latest.price
}

func latestStop(tree *dmt.Tree, symbol string) *datura.Artifact {
	var latest *datura.Artifact

	for artifact := range tree.Seek([]byte("stoploss/" + symbol)) {
		if latest != nil && latest.Timestamp() > artifact.Timestamp() {
			continue
		}

		latest = artifact
	}

	return latest
}
