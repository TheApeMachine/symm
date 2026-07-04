package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type Book struct {
	Channel string     `json:"channel"`
	Type    string     `json:"type"`
	Data    []BookData `json:"data"`
}

type BookData struct {
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Checksum  uint32      `json:"checksum"`
	Timestamp time.Time   `json:"timestamp"`
}

type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

type BookDataSlice []BookData

func NewBookDataSlice(buf []byte) BookDataSlice {
	data := BookDataSlice{}
	errnie.Error(sonic.Unmarshal(buf, &data))
	return data
}
