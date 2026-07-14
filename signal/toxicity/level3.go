package toxicity

import (
	"context"
	"iter"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Level3 reads the SDK's managed level3 order books for the shared toxicity
sample. The BookManager applies and checksums every snapshot and delta, so
Level3 does not decode raw frames itself.
*/
type Level3 struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  []kraken.Level3Data
}

/*
NewLevel3 constructs the toxicity level3 book iterator for api.
*/
func NewLevel3(ctx context.Context, api *websocket.API) *Level3 {
	if api == nil {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)

	level3 := &Level3{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  []kraken.Level3Data{},
	}

	level3.api.On("level3", level3.On)

	return level3
}

/*
On retains L3 rows from the shared Kraken transport so the next Thesis can
advance the existing manifold from the authoritative event stream.
*/
func (level3 *Level3) On(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewLevel3(data)

	if len(frame.Data) == 0 {
		return
	}

	level3.cache = append(level3.cache, frame.Data...)
}

/*
Rows transfers the current L3 event batch to one Thesis and leaves the next
tick with an empty batch.
*/
func (level3 *Level3) Rows() []kraken.Level3Data {
	rows := level3.cache
	level3.cache = level3.cache[:0]

	return rows
}

/*
Books returns every order book the SDK currently manages for this transport.
*/
func (level3 *Level3) Books() iter.Seq[*spot.BookManager] {
	return level3.api.Books()
}
