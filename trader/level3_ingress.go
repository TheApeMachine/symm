package trader

import (
	"context"
	"iter"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
Level3Ingress buffers authoritative websocket rows and drains them into the tick
Thesis before signal measurement on each cognitive epoch.
*/
type Level3Ingress struct {
	ring    *structure.MPMCRing[kraken.Level3Data]
	books   iter.Seq[*spot.BookManager]
	enabled bool
}

func NewLevel3Ingress(
	ctx context.Context,
	api *websocket.API,
) (*Level3Ingress, error) {
	capacity := viper.GetInt("market.l3_ring_capacity")
	ring, err := structure.NewMPMCRing[kraken.Level3Data](ctx, capacity)

	if err != nil {
		return nil, errnie.Error(err)
	}

	ingress := &Level3Ingress{
		ring:    ring,
		books:   api.Books(),
		enabled: viper.GetBool("market.l3_enabled"),
	}

	if ingress.enabled {
		api.On("level3", ingress.On)
	}

	return ingress, nil
}

func (ingress *Level3Ingress) On(data []byte) {
	for _, row := range kraken.NewLevel3DataSlice(data) {
		if row.Symbol == "" {
			continue
		}

		if !ingress.ring.Push(row) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"trader: level3 ingress ring full",
				nil,
			))
		}
	}
}

/*
Drain applies every buffered row to the analyzer on the current tick thesis.
*/
func (ingress *Level3Ingress) Drain(
	thesis *types.Thesis,
	analyzer *logic.Analyzer,
	instrument *Instrument,
) {
	if !ingress.enabled || analyzer == nil || thesis == nil {
		return
	}

	book := NewSDKLevel3Book(ingress.books)

	for {
		row := ingress.ring.Pop()

		if row.Symbol == "" {
			return
		}

		pair, err := instrument.Pair(row.Symbol)

		if err != nil {
			errnie.Error(err)
			continue
		}

		analyzer.IngestLevel3(
			thesis,
			row,
			pair.PricePrecision,
			pair.QtyPrecision,
			book.ForSymbol(row.Symbol),
		)
	}
}
