package toxicity

import (
	"context"
	"iter"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
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
}

/*
NewLevel3 constructs the toxicity level3 book iterator for api.
*/
func NewLevel3(ctx context.Context, api *websocket.API) (*Level3, error) {
	if api == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"toxicity: websocket API required",
			nil,
		))
	}

	ctx, cancel := context.WithCancel(ctx)

	return &Level3{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
	}, nil
}

/*
Books returns every order book the SDK currently manages for this transport.
*/
func (level3 *Level3) Books() iter.Seq[*spot.BookManager] {
	return level3.api.Books()
}
