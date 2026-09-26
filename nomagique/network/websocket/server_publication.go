package websocket

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/ui"
)

/*
	socketOutput retains current component properties while its socket is busy.

Raw frames remain ordered events; bindings are replaceable property state. The
pending state is bounded by the authored properties, not the arrival count.
The owning WebSocketServer capability serializes access with publicationMu.
*/
type socketOutput struct {
	frames   chan []byte
	wake     chan struct{}
	bindings map[[3]string]string
}

/*
	Publish merges component properties atomically and wakes each socket writer.

A later value supersedes only the same graph/component/property. Slow sockets
retain other changed properties and never block market processing.
*/
func (server *WebSocketServerServer) Publish(ctx context.Context, call ui.Receiver_publish) error {
	data, err := call.Args().Data()
	if err != nil {
		return errnie.Error(err)
	}
	message, err := capnp.Unmarshal(data)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "websocket: decode UI bindings", err))
	}
	defer message.Release()
	bindings, err := ui.ReadRootBindings(message)
	if err != nil {
		return errnie.Error(err)
	}
	values, err := bindings.Values()
	if err != nil {
		return errnie.Error(err)
	}
	updates := make(map[[3]string]string, values.Len())
	for index := range values.Len() {
		bound := values.At(index)
		graph, err := bound.Graph()
		if err != nil {
			return errnie.Error(err)
		}
		component, err := bound.Component()
		if err != nil {
			return errnie.Error(err)
		}
		prop, err := bound.Prop()
		if err != nil {
			return errnie.Error(err)
		}
		value, err := bound.Value()
		if err != nil {
			return errnie.Error(err)
		}
		updates[[3]string{graph, component, prop}] = value
	}
	server.publicationMu.Lock()
	defer server.publicationMu.Unlock()
	if server.publications == nil {
		server.publications = make(map[[3]string]string)
	}
	for key, value := range updates {
		server.publications[key] = value
	}
	server.clients.Range(func(_, value any) bool {
		peer := value.(*socketOutput)
		for key, value := range updates {
			peer.bindings[key] = value
		}
		select {
		case peer.wake <- struct{}{}:
		default:
		}
		return true
	})
	return nil
}

/* pending transfers changed properties into one native Cap'n Proto frame. */
func (server *WebSocketServerServer) pending(peer *socketOutput) ([]byte, error) {
	server.publicationMu.Lock()
	updates := peer.bindings
	peer.bindings = make(map[[3]string]string)
	server.publicationMu.Unlock()
	if len(updates) == 0 {
		return nil, nil
	}
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, errnie.Error(err)
	}
	defer message.Release()
	bindings, err := ui.NewRootBindings(segment)
	if err != nil {
		return nil, errnie.Error(err)
	}
	values, err := bindings.NewValues(int32(len(updates)))
	if err != nil {
		return nil, errnie.Error(err)
	}
	index := 0
	for key, value := range updates {
		bound := values.At(index)
		for _, err := range []error{bound.SetGraph(key[0]), bound.SetComponent(key[1]), bound.SetProp(key[2]), bound.SetValue(value)} {
			if err != nil {
				return nil, errnie.Error(err)
			}
		}
		index++
	}
	output, err := message.Marshal()
	return output, errnie.Error(err)
}
