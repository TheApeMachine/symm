package ui

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
UIComponentServer holds one authored UI component for the duration of a single
evaluation. The component identity is a plain name resolved against the
generated frontend component registry, so adding a component to the React
library never requires touching this schema.
*/
type UIComponentServer struct {
	component Component
}

func NewUIComponent() *UIComponentServer {
	return &UIComponentServer{}
}

func (server *UIComponentServer) Write(ctx context.Context, call UIComponent_write) error {
	name, err := call.Args().Name()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: read component name failed",
			err,
		))
	}

	className, err := call.Args().ClassName()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: read component className failed",
			err,
		))
	}

	propsJson, err := call.Args().PropsJson()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: read component propsJson failed",
			err,
		))
	}

	children, err := call.Args().Components()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: read child components failed",
			err,
		))
	}

	component, err := newDetachedComponent()
	if err != nil {
		return err
	}

	if err := component.SetName(name); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: set component name failed",
			err,
		))
	}

	if err := component.SetClassName(className); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: set component className failed",
			err,
		))
	}

	if err := component.SetPropsJson(propsJson); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: set component propsJson failed",
			err,
		))
	}

	if children.IsValid() {
		if err := component.SetComponents(children); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"ui: set child components failed",
				err,
			))
		}
	}

	server.component = component
	return nil
}

func (server *UIComponentServer) Done(ctx context.Context, call UIComponent_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: alloc component results failed",
			err,
		))
	}

	if err := results.SetOut(server.component); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: set component result failed",
			err,
		))
	}

	server.component = Component{}
	return nil
}

/*
newDetachedComponent allocates a component in its own message so the value
outlives the incoming call arguments, which Cap'n Proto releases once Write
returns.
*/
func newDetachedComponent() (Component, error) {
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return Component{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: new component message failed",
			err,
		))
	}

	component, err := NewRootComponent(segment)
	if err != nil {
		return Component{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: new root component failed",
			err,
		))
	}

	return component, nil
}
