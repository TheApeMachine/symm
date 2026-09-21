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

	if err := nest(ctx, component, children); err != nil {
		return err
	}

	server.component = component
	return nil
}

/*
nest renders each wired child and places it under its parent.

Children are capabilities rather than values, so a parent asks each one what it
is at the moment it is assembled. That is what makes the authored graph's
parent/child relationships the rendered nesting.
*/
func nest(ctx context.Context, parent Component, children UIComponent_List) error {
	if !children.IsValid() || children.Len() == 0 {
		return nil
	}

	nested, err := parent.NewComponents(int32(children.Len()))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: allocate child components failed",
			err,
		))
	}

	for index := range children.Len() {
		child, err := children.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"ui: read child component failed",
				err,
			))
		}

		if err := renderInto(ctx, child, nested, index); err != nil {
			return err
		}
	}

	return nil
}

/*
renderInto asks one child component what it currently is and places it under
its parent while the answer is still alive, because a Cap'n Proto result does
not outlive the call that produced it.
*/
func renderInto(
	ctx context.Context,
	child UIComponent,
	nested Component_List,
	index int,
) error {
	future, release := child.Done(ctx, nil)
	defer release()

	results, err := future.Struct()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"ui: child component did not render",
			err,
		))
	}

	rendered, err := results.Out()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ui: child component carried no result",
			err,
		))
	}

	if err := nested.Set(index, rendered); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ui: place child component failed",
			err,
		))
	}

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
