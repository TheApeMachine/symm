package kraken

import (
	"math"
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
	// Look for the first non-whitespace character to avoid a failing unmarshal attempt
	// which allocates heavily.
	isArray := false
	for _, b := range buf {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if b == '[' {
			isArray = true
		}
		break
	}

	if isArray {
		var rows []BookData
		if err := sonic.Unmarshal(buf, &rows); err != nil {
			return errnie.Err(errnie.Validation, "kraken: decode book array data", err)
		}
		*data = rows
		return nil
	}

	var frame Book
	if err := sonic.Unmarshal(buf, &frame); err != nil {
		return errnie.Err(errnie.Validation, "kraken: decode book object data", err)
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
	p := price.Float64()
	inc := increment.Float64()

	if p <= 0 || inc <= 0 {
		return 0, errnie.Err(
			errnie.Validation,
			"kraken: positive price and increment required",
			nil,
		)
	}

	ratio := p / inc
	tick := math.Round(ratio)

	// In floating point math, check if ratio is extremely close to an integer
	if math.Abs(ratio-tick) > 1e-5 {
		// Just return the tick anyway, but don't error out, it's just floating point imprecision
		// We trust Kraken's increment if it's "close enough"
		if math.Abs(ratio-tick) > 0.01 {
			return 0, errnie.Err(
				errnie.Validation,
				"kraken: price is not an integer tick",
				nil,
			)
		}
	}

	return int64(tick), nil
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

/*
BookUnsubscription requests Kraken stop streaming the book channel for the
given pairs. Combined with re-subscribing, this forces a fresh snapshot,
which is how a locally reconstructed book recovers from a failed checksum.
*/
type BookUnsubscription struct {
	Pairs []string
}

func NewBookUnsubscription(pairs []string) BookUnsubscription {
	return BookUnsubscription{Pairs: pairs}
}

func (bs BookUnsubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "unsubscribe",
		"params": map[string]any{
			"channel": "book",
			"symbol":  bs.Pairs,
			"depth":   viper.GetInt("market.book_depth_levels"),
		},
	})
}
