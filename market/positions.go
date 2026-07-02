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
change are computed here only when both entry and mark exist so nothing
downstream invents them. Returns nil when no balances have been published yet —
"no ledger" stays distinct from "flat".
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
		reading := map[string]any{
			"symbol":    position.symbol,
			"asset":     position.asset,
			"quote":     quote,
			"quantity":  position.quantity,
			"updatedAt": balances.Timestamp(),
		}

		mark := latestMark(tree, position.symbol)
		if mark > 0 {
			reading["mark"] = mark
			reading["value"] = mark * position.quantity
		} else {
			reading["status"] = "missing_mark"
		}

		entry, entryErr := latestEntryPrice(tree, position.symbol)
		if entryErr != nil {
			return nil, entryErr
		}
		if entry > 0 {
			reading["entry"] = entry
		} else if reading["status"] == nil {
			reading["status"] = "missing_entry"
		} else {
			reading["status"] = "missing_mark_entry"
		}

		if mark > 0 && entry > 0 {
			reading["unrealizedPnl"] = (mark - entry) * position.quantity
			reading["changePct"] = (mark - entry) / entry * 100
			reading["status"] = "marked"
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
		asset := balanceAsset(balances, rowIndex)

		if asset == "" {
			break
		}

		quantity := balanceQuantity(balances, rowIndex)

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

func balanceAsset(balances *datura.Artifact, rowIndex int) string {
	if asset := datura.Peek[string](balances, "data", rowIndex, "asset"); asset != "" {
		return asset
	}

	return datura.Peek[string](balances, "asset", rowIndex, "asset")
}

func balanceQuantity(balances *datura.Artifact, rowIndex int) float64 {
	if quantity := datura.Peek[float64](balances, "data", rowIndex, "balance"); quantity > 0 {
		return quantity
	}

	return datura.Peek[float64](balances, "asset", rowIndex, "balance")
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
	if tree == nil {
		return 0
	}

	target := strings.ToUpper(strings.TrimSpace(symbol))
	if target == "" {
		return 0
	}

	if mark := latestTickerMark(tree, target); mark > 0 {
		return mark
	}

	if mark := tickerPrefixMark(tree, []byte("ticker/"+target+"/"), target); mark > 0 {
		return mark
	}

	return tickerPrefixMark(tree, []byte("ticker/"), target)
}

func latestTickerMark(tree *dmt.Tree, target string) float64 {
	wire, ok := tree.Get([]byte("latest/ticker/" + target))
	if !ok || len(wire) == 0 {
		return 0
	}

	artifact := datura.Acquire("", datura.APPJSON)
	defer artifact.Release()

	if _, err := artifact.Unpack(wire); err != nil {
		return 0
	}

	symbol := datura.Peek[string](artifact, "data", 0, "symbol")
	if strings.ToUpper(symbol) != target {
		return 0
	}

	return datura.Peek[float64](artifact, "data", 0, "last")
}

func tickerPrefixMark(tree *dmt.Tree, prefix []byte, target string) float64 {
	type stampedMark struct {
		price float64
		stamp int64
	}

	var latest stampedMark

	for artifact := range tree.Seek(prefix) {
		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")

			if symbol == "" {
				break
			}

			if strings.ToUpper(symbol) != target {
				continue
			}

			price := datura.Peek[float64](artifact, "data", rowIndex, "last")
			if price <= 0 {
				continue
			}

			stamp := artifact.Timestamp()

			if latest.stamp > stamp {
				continue
			}

			latest = stampedMark{price: price, stamp: stamp}
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
