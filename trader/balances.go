package trader

import (
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
)

type balanceRow struct {
	Asset   string  `json:"asset"`
	Balance float64 `json:"balance"`
}

type balanceFrame struct {
	Data  []balanceRow `json:"data"`
	Asset []balanceRow `json:"asset"`
}

func balanceRows(artifact *datura.Artifact) []balanceRow {
	if artifact == nil {
		return nil
	}

	payload := artifact.DecryptPayload()
	if len(payload) == 0 {
		return nil
	}

	var frame balanceFrame
	if err := sonic.Unmarshal(payload, &frame); err != nil {
		return nil
	}

	if len(frame.Data) > 0 {
		return frame.Data
	}

	return frame.Asset
}

func holdsSymbol(rows []balanceRow, symbol string) bool {
	base, _, _ := strings.Cut(symbol, "/")
	base = strings.ToUpper(strings.TrimSpace(base))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	if base == "" && symbol == "" {
		return false
	}

	for _, row := range rows {
		asset := strings.ToUpper(strings.TrimSpace(row.Asset))
		if asset != symbol && asset != base {
			continue
		}

		return row.Balance > 0
	}

	return false
}
