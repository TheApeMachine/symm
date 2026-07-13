package toxicity

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/utils"
)

/*
Level3 ingests level3 order events into the shared toxicity sample.
*/
type Level3 struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	cache  []book.Book
}

func NewLevel3(ctx context.Context, api *websocket.API) *Level3 {
	ctx, cancel := context.WithCancel(ctx)

	level3 := &Level3{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		cache:  []book.Book{},
	}

	level3.api.On("level3", level3.On)
	return level3
}

func (level3 *Level3) On(data []byte) {
	if len(data) == 0 {
		return
	}

	level3.cache = append(
		level3.cache, utils.Unmarshal[book.Book](data),
	)
}
