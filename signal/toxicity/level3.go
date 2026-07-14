package toxicity

import (
	"iter"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Level3 exposes the SDK-managed books consumed by toxicity measurements.
*/
type Level3 struct {
	api *websocket.API
}

/*
NewLevel3 constructs the toxicity level3 book iterator for api.
*/
func NewLevel3(api *websocket.API) *Level3 {
	if api == nil {
		return nil
	}

	return &Level3{api: api}
}

/*
Books returns every order book the SDK currently manages for this transport.
*/
func (level3 *Level3) Books() iter.Seq[*spot.BookManager] {
	return level3.api.Books()
}
