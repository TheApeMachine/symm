package kraken

import (
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
	Symbol         string           `json:"symbol"`
	Type           string           `json:"type"`
	PriceIncrement *decimal.Decimal `json:"-"`
	Bids           []BookLevel      `json:"bids"`
	Asks           []BookLevel      `json:"asks"`
	Checksum       uint32           `json:"checksum"`
	Timestamp      time.Time        `json:"timestamp"`
}

type BookLevel struct {
	Price decimal.Decimal `json:"price"`
	Qty   float64         `json:"qty"`
}

func NewBook(buf []byte) *Book {
	var book Book

	if err := sonic.Unmarshal(buf, &book); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid book",
			err,
		))
	}

	for index := range book.Data {
		if book.Data[index].Type == "" {
			book.Data[index].Type = book.Type
		}
	}

	return &book
}

func (book *Book) IsSuccess() bool {
	return len(book.Data) > 0
}

func (book *Book) Action() string {
	return "book"
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
			"depth":   viper.GetInt("market.book.depth"),
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

/*
Level3Unsubscription requests a fresh authenticated Level 3 snapshot for the
specified pairs when the resident checksum diverges from the venue.
*/
type Level3Unsubscription struct {
	Pairs []string
	Token string
}

func NewLevel3Unsubscription(pairs []string, token string) Level3Unsubscription {
	return Level3Unsubscription{Pairs: pairs, Token: token}
}

func (unsubscription Level3Unsubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "unsubscribe",
		"params": map[string]any{
			"channel": "level3",
			"symbol":  unsubscription.Pairs,
			"token":   unsubscription.Token,
		},
	})
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
			"depth":   viper.GetInt("market.book.depth"),
		},
	})
}
