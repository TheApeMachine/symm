package toxicity

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Level3 exposes the SDK-managed books consumed by toxicity measurements through
the API read lease.
*/
type Level3 struct {
	api *websocket.API
}

/*
NewLevel3 constructs the toxicity level3 book accessor for api.
*/
func NewLevel3(api *websocket.API) *Level3 {
	if api == nil {
		return nil
	}

	return &Level3{api: api}
}

/*
PeekBook invokes fn under the Level3 read lease for symbol.
*/
func (level3 *Level3) PeekBook(symbol string, fn func(*book.Book)) bool {
	if level3 == nil || level3.api == nil {
		return false
	}

	return level3.api.PeekBook(symbol, fn)
}
