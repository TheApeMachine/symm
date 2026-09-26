package compiler

import (
	"context"
	"errors"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* Write binds the immutable upstream Text selector for partitioned execution. */
func (factory *stageFactory) Write(ctx context.Context, call runtime.StageFactory_write) error {
	producer, err := call.Args().Producer()

	if err != nil {
		return errnie.Error(err)
	}
	node, err := call.Args().Node()

	if err != nil {
		return errnie.Error(err)
	}
	field, err := call.Args().Field()

	if err != nil {
		return errnie.Error(err)
	}

	if (producer != "" || node != "" || field != "") && (producer == "" || node == "" || field == "") {
		return errnie.Error(errnie.Err(errnie.Validation, "factory: partition requires producer, node and field", nil))
	}

	if factory.producer != "" && (producer != factory.producer || node != factory.node || field != factory.field) {
		return errnie.Error(errnie.Err(errnie.Validation, "factory: partition selector cannot change", nil))
	}
	factory.producer, factory.node, factory.field = producer, node, field
	return nil
}

/* Done reports the number of independently owned graph capabilities. */
func (factory *stageFactory) Done(ctx context.Context, call runtime.StageFactory_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	result.SetPartitions(uint64(len(factory.children)))
	return nil
}

/* Step selects an existing child or creates it once, without scheduling work. */
func (factory *stageFactory) Step(ctx context.Context, call runtime.StageNode_step) error {
	key, err := factory.partition(call.Args())

	if err != nil {
		return err
	}
	if key == "" {
		return nil
	}
	child, found := factory.children[key]

	if !found {
		child, err = factory.create()

		if err != nil {
			return err
		}
		factory.children[key] = child
	}
	future, release := child.Step(ctx, func(params runtime.StageNode_step_Params) error {
		return capnp.Struct(params).CopyFrom(capnp.Struct(call.Args()))
	})
	defer release()
	result, err := future.Struct()

	if err != nil {
		return errnie.Error(err)
	}
	allocated, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(capnp.Struct(allocated).CopyFrom(capnp.Struct(result)))
}

/* partition reads the declared native Text field; absent keys never share state. */
func (factory *stageFactory) partition(args runtime.StageNode_step_Params) (string, error) {
	if factory.producer == "" {
		return "", errnie.Error(errnie.Err(errnie.Validation, "factory: partition selector is not configured", nil))
	}
	upstream, err := args.Upstream()

	if err != nil {
		return "", errnie.Error(err)
	}
	for index := range upstream.Len() {
		result := upstream.At(index)
		producer, err := result.Producer()

		if err != nil {
			return "", errnie.Error(err)
		}
		node, err := result.Node()

		if err != nil {
			return "", errnie.Error(err)
		}

		if producer != factory.producer || node != factory.node {
			continue
		}

		if result.Epoch() != args.Epoch() || result.Sequence() != args.Sequence() {
			return "", errnie.Error(errnie.Err(errnie.Validation, "factory: partition key belongs to another observation", nil))
		}
		reflected, err := ReflectInterface(result.InterfaceId())

		if err != nil {
			return "", err
		}
		field, found := resolveOutputField(reflected, factory.field)

		if !found || field.Which != schema.Type_Which_text {
			return "", errnie.Error(errnie.Err(errnie.Validation, "factory: partition selector must name a Text output", nil))
		}
		pointer, err := result.Value()

		if err != nil {
			return "", errnie.Error(err)
		}
		value := pointer.Struct()

		if field.InUnion && value.Uint16(capnp.DataOffset(field.DiscriminantOffset*2)) != field.DiscriminantValue {
			return "", errnie.Error(errnie.Err(errnie.Validation, "factory: partition key is not active", nil))
		}
		text, err := value.Ptr(uint16(field.Offset))

		if err != nil {
			return "", errnie.Error(err)
		}

		if text.Text() == "" {
			return "", errnie.Error(errnie.Err(errnie.Validation, "factory: partition key is empty", nil))
		}
		return text.Text(), nil
	}
	return "", nil
}

/* Fence reaches every child, including markets quiet at the time of the fence. */
func (factory *stageFactory) Fence(ctx context.Context, call runtime.StageNode_fence) error {
	var failures []error
	for _, child := range factory.children {
		future, release := child.Fence(ctx, nil)
		_, err := future.Struct()
		release()

		if err != nil {
			failures = append(failures, errnie.Error(err))
		}
	}
	return errors.Join(failures...)
}

/* Shutdown releases all owned graph capabilities. */
func (factory *stageFactory) Shutdown() {
	for _, child := range factory.children {
		child.Release()
	}
}
