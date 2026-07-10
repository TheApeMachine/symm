package broker

import (
	"slices"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Mark struct {
	desk *Desk
	ui   chan []byte
}

func NewMark(desk *Desk, ui chan []byte) *Mark {
	return &Mark{
		desk: desk,
		ui:   ui,
	}
}

func (mark *Mark) On(data []byte) {
	updated := false

	for _, ticker := range kraken.NewTickerDataSlice(data) {
		position, ok := mark.desk.positions.Load(ticker.Symbol)

		if !ok {
			continue
		}

		position.(*Position).AddTicker(&ticker)
		updated = true
	}

	if !updated || mark.ui == nil {
		return
	}

	positions := make([]*PositionData, 0)

	mark.desk.positions.Range(func(key any, value any) bool {
		position := value.(*Position)

		if slices.Contains(
			[]types.Status{types.CLOSED, types.FATAL}, position.status,
		) {
			mark.desk.positions.Delete(key)
			return true
		}

		positions = append(positions, position.data)
		return true
	})

	mark.desk.refreshStatus()

	if len(positions) == 0 {
		return
	}

	if mark.ui != nil {
		select {
		case mark.ui <- datura.Map[any]{
			"positions": positions,
		}.Marshal():
		default:
		}
	}
}
