package kraken

import (
	"math/big"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

type Book struct {
	Channel string     `json:"channel"`
	Type    string     `json:"type"`
	Data    []BookData `json:"data"`
}

type BookData struct {
	Symbol         string          `json:"symbol"`
	Type           string          `json:"type"`
	PriceIncrement decimal.Decimal `json:"-"`
	Bids           []BookLevel     `json:"bids"`
	Asks           []BookLevel     `json:"asks"`
	Checksum       uint32          `json:"checksum"`
	Timestamp      time.Time       `json:"timestamp"`
}

type BookLevel struct {
	Price decimal.Decimal `json:"price"`
	Qty   float64         `json:"qty"`
}

type BookDataSlice []BookData

func NewBookDataSlice(buf []byte) BookDataSlice {
	data := BookDataSlice{}
	errnie.Error(data.Decode(buf))
	return data
}

func (data *BookDataSlice) Decode(buf []byte) error {
	var rows []BookData
	if err := sonic.Unmarshal(buf, &rows); err == nil {
		*data = rows
		return nil
	}

	var frame Book
	if err := sonic.Unmarshal(buf, &frame); err != nil {
		return errnie.Err(errnie.Validation, "kraken: decode book data", err)
	}

	for index := range frame.Data {
		if frame.Data[index].Type == "" {
			frame.Data[index].Type = frame.Type
		}
	}

	*data = frame.Data
	return nil
}

func PriceTick(price decimal.Decimal, increment decimal.Decimal) (int64, error) {
	if !decimalPositive(price) || !decimalPositive(increment) {
		return 0, errnie.Err(
			errnie.Validation,
			"kraken: positive price and increment required",
			nil,
		)
	}

	ratio := new(big.Rat).Quo(price.Rat(), increment.Rat())

	if !ratio.IsInt() {
		return 0, errnie.Err(
			errnie.Validation,
			"kraken: price is not an integer tick",
			nil,
		)
	}

	tick := ratio.Num()

	if !tick.IsInt64() {
		return 0, errnie.Err(
			errnie.Validation,
			"kraken: price tick is out of int64 range",
			nil,
		)
	}

	return tick.Int64(), nil
}

type BookSubscription struct {
	Channel string   `json:"channel"`
	Type    string   `json:"type"`
	Pairs   []string `json:"pairs"`
}

func NewBookSubscription(pairs []string) BookSubscription {
	return BookSubscription{
		Channel: "book",
		Type:    "subscribe",
		Pairs:   pairs,
	}
}

func (bs BookSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": bs.Channel,
			"symbol":  bs.Pairs,
			"depth":   viper.GetInt("market.book_depth_levels"),
		},
	})
}
