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
	Type      string      `json:"type"`
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
