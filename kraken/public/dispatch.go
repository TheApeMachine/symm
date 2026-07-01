package public

import (
	"fmt"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/types"
)

func DispatchWebSocketFrame(
	origin string,
	handlers map[string]types.Socket,
	message []byte,
) (*datura.Artifact, error) {
	socketMessage := types.Acquire()
	defer socketMessage.Release()

	if err := socketMessage.Decode(message); err != nil {
		return nil, fmt.Errorf("%s: websocket decode: %w", origin, err)
	}

	if socketMessage.Channel == "" {
		return nil, nil
	}

	handler, ok := handlers[socketMessage.Channel]
	if !ok || handler == nil {
		return nil, nil
	}

	artifact := datura.Acquire(origin, datura.APPJSON).
		WithPayload(message)
	artifact.SetTimestamp(time.Now().UTC().UnixNano())

	return handler.Send(artifact), nil
}
