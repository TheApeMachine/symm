package http

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/ui"
)

/* Publish accepts actual component bindings from graph Consumer capabilities. */
func (server *HTTPServerServer) Publish(ctx context.Context, call ui.Receiver_publish) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http: read UI bindings", err))
	}
	message, err := capnp.Unmarshal(data)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http: decode UI bindings", err))
	}
	defer message.Release()
	bindings, err := ui.ReadRootBindings(message)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http: bindings root", err))
	}
	values, err := bindings.Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http: bindings values", err))
	}
	server.publicationMu.Lock()
	defer server.publicationMu.Unlock()

	if server.publications == nil {
		server.publications = make(map[[3]string]string)
	}
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
		server.publications[[3]string{graph, component, prop}] = value
	}
	server.wsServer.Broadcast(data)
	return nil
}

/* replayBindings supplies retained component values when a browser joins. */
func (server *HTTPServerServer) replayBindings() {
	server.publicationMu.Lock()
	defer server.publicationMu.Unlock()

	if len(server.publications) == 0 {
		return
	}
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		errnie.Error(err)
		return
	}
	defer message.Release()
	bindings, err := ui.NewRootBindings(segment)

	if err != nil {
		errnie.Error(err)
		return
	}
	values, err := bindings.NewValues(int32(len(server.publications)))

	if err != nil {
		errnie.Error(err)
		return
	}
	index := 0
	for key, value := range server.publications {
		bound := values.At(index)
		for _, err := range []error{bound.SetGraph(key[0]), bound.SetComponent(key[1]), bound.SetProp(key[2]), bound.SetValue(value)} {
			if err != nil {
				errnie.Error(err)
				return
			}
		}
		index++
	}
	data, err := message.Marshal()

	if err != nil {
		errnie.Error(err)
		return
	}
	server.wsServer.Broadcast(data)
}
