package kraken

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type OpenPositions struct {
	Error  []string                `json:"error"`
	Result map[string]OpenPosition `json:"result"`
}

type OpenPosition struct {
	OrderTxID  string          `json:"ordertxid"`
	PosStatus  string          `json:"posstatus"`
	Pair       string          `json:"pair"`
	Time       float64         `json:"time"`
	Type       string          `json:"type"`
	OrderType  string          `json:"ordertype"`
	Cost       decimal.Decimal `json:"cost"`
	Fee        decimal.Decimal `json:"fee"`
	Vol        decimal.Decimal `json:"vol"`
	VolClosed  decimal.Decimal `json:"vol_closed"`
	Margin     decimal.Decimal `json:"margin"`
	Value      decimal.Decimal `json:"value"`
	Net        decimal.Decimal `json:"net"`
	Terms      decimal.Decimal `json:"terms"`
	RolloverTm string          `json:"rollovertm"`
	Misc       string          `json:"misc"`
	OFlags     string          `json:"oflags"`
}

func NewOpenPositions(buf []byte) *OpenPositions {
	var positions OpenPositions

	if err := sonic.Unmarshal(buf, &positions); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"failed to unmarshal open positions",
			err,
		))
		return nil
	}

	return &positions
}
