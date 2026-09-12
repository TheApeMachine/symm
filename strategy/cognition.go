package strategy

import (
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
ask drives one cognition engine command and reads its single result.
*/
func ask(engine core.Primitive, command *cognition.Command) (cognition.Result, error) {
	evaluation := transport.NewEvaluate(engine)
	var result cognition.Result

	for out := range evaluation.Next(transport.NewOne(unsafe.Pointer(command)).Next(nil)) {
		result = *(*cognition.Result)(out)
	}

	return result, evaluation.Error()
}

/*
observeContext records one association into the engine's memory.
*/
func observeContext(engine core.Primitive, assoc cognition.Association) error {
	_, err := ask(engine, &cognition.Command{Observe: &assoc})

	return err
}

/*
evaluateContext classifies one context.
*/
func evaluateContext(engine core.Primitive, context []byte) (cognition.Evaluation, error) {
	result, err := ask(engine, &cognition.Command{Evaluate: &cognition.Question{Context: context}})

	return result.Evaluation, err
}

/*
engineTree reads the engine's current immutable trie.
*/
func engineTree(engine core.Primitive) (*iradix.Tree[[]byte], error) {
	result, err := ask(engine, &cognition.Command{Root: &cognition.Root{}})

	return result.Tree, err
}

/*
classCensus reads how often each class has been observed.
*/
func classCensus(engine core.Primitive) (map[string]int32, error) {
	result, err := ask(engine, &cognition.Command{Census: &cognition.Census{}})

	return result.Classes, err
}

/*
basinOf parses one stored key into the class and context it names.
*/
func basinOf(key []byte) (class, context []byte, valid bool) {
	keys := cognition.NewKey()
	command := cognition.KeyCommand{Parse: key}

	for out := range keys.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
		result := *(*cognition.KeyResult)(out)
		return result.Class, result.Context, result.Valid
	}

	if err := keys.Error(); err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "recognition: unreadable basin key", err))
	}

	return nil, nil, false
}

/*
packedWeight unpacks one stored weight record.
*/
func packedWeight(record []byte) cognition.PackedWeight {
	unpack := cognition.NewWeight()
	held := cognition.WeightRecord(record)

	for out := range unpack.Next(transport.NewOne(unsafe.Pointer(&held)).Next(nil)) {
		return *(*cognition.PackedWeight)(out)
	}

	if err := unpack.Error(); err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "recognition: unreadable packed weight", err))
	}

	return cognition.PackedWeight{}
}

/*
askGrid drives one grid command and reads its single result.
*/
func askGrid(space core.Primitive, command *grid.Command) (grid.Result, error) {
	evaluation := transport.NewEvaluate(space)
	var result grid.Result

	for out := range evaluation.Next(transport.NewOne(unsafe.Pointer(command)).Next(nil)) {
		result = *(*grid.Result)(out)
	}

	return result, evaluation.Error()
}

/*
askLearner drives one associative learner command and reads its single result.
*/
func askLearner(learner core.Primitive, command *associative.Command) (associative.Result, error) {
	evaluation := transport.NewEvaluate(learner)
	var result associative.Result

	for out := range evaluation.Next(transport.NewOne(unsafe.Pointer(command)).Next(nil)) {
		result = *(*associative.Result)(out)
	}

	return result, evaluation.Error()
}

/*
spaceState reads the grid's labels, width, formation and version.
*/
func spaceState(space core.Primitive) grid.Result {
	result, err := askGrid(space, &grid.Command{State: &grid.State{}})

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "recognition: unreadable grid state", err))
		return grid.Result{}
	}

	return result
}

/*
tokenQuantity recovers a condition token's quantity identity.
*/
func tokenQuantity(token uint64) uint64 {
	tokens := grid.NewToken()
	command := grid.TokenCommand{Quantity: &grid.TokenQuantity{Token: token}}

	for out := range tokens.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
		return (*grid.TokenResult)(out).Quantity
	}

	if err := tokens.Error(); err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "recognition: unreadable condition token", err))
	}

	return 0
}

/*
cloneMeasurement yields one independent deep copy of a measurement through the
canonical cloner primitive.
*/
func cloneMeasurement(measured *data.Measurement[float64]) *data.Measurement[float64] {
	cloner := data.NewCloner[float64]()
	var clone *data.Measurement[float64]

	for out := range cloner.Next(transport.NewOne(unsafe.Pointer(&measured)).Next(nil)) {
		clone = *(**data.Measurement[float64])(out)
	}

	if err := cloner.Error(); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "agent: measurement clone failed", err))
		return nil
	}

	return clone
}

/*
retainedTree reads the latest value a retained primitive holds.
*/
func retainedTree(memory core.Primitive) *iradix.Tree[[]byte] {
	var tree *iradix.Tree[[]byte]

	for out := range memory.Next(nil) {
		tree = *(**iradix.Tree[[]byte])(out)
	}

	if err := memory.Error(); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "recognition: unreadable retained memory", err))
		return nil
	}

	return tree
}

/*
ringCommand drives one ring command and reads its single result.
*/
func ringCommand[T any](ring core.Primitive, command *store.RingCommand[T]) (store.RingResult[T], error) {
	evaluation := transport.NewEvaluate(ring)
	var result store.RingResult[T]

	for out := range evaluation.Next(transport.NewOne(unsafe.Pointer(command)).Next(nil)) {
		result = *(*store.RingResult[T])(out)
	}

	return result, evaluation.Error()
}

/*
ringWrite appends one value to a ring.
*/
func ringWrite[T any](ring core.Primitive, value T) error {
	_, err := ringCommand(ring, &store.RingCommand[T]{
		Write: &store.RingValue[T]{Value: &value},
	})

	return err
}

/*
ringWriteSlot writes one value, or one nested child ring, into a slot.
*/
func ringWriteSlot[T any](ring core.Primitive, slot int, value *T, child core.Primitive) error {
	_, err := ringCommand(ring, &store.RingCommand[T]{
		WriteAt: &store.RingSlot[T]{
			Value: store.RingValue[T]{Value: value, Child: child},
			Slot:  slot,
		},
	})

	return err
}

/*
ringLen reads the ring's parent length.
*/
func ringLen[T any](ring core.Primitive) int {
	result, err := ringCommand[T](ring, &store.RingCommand[T]{Len: true})

	if err != nil {
		return 0
	}

	return result.Len
}

/*
ringAdvance moves the ring to the next child sequence.
*/
func ringAdvance[T any](ring core.Primitive) {
	_, _ = ringCommand[T](ring, &store.RingCommand[T]{Advance: true})
}

/*
ringCurrent reads the parent's current element.
*/
func ringCurrent[T any](ring core.Primitive) (T, bool) {
	result, err := ringCommand[T](ring, &store.RingCommand[T]{Current: true})

	if err != nil {
		var zero T
		return zero, false
	}

	return result.Value, result.Has
}

/*
ringChildLen reads the current child sequence's length.
*/
func ringChildLen[T any](ring core.Primitive) int {
	result, err := ringCommand[T](ring, &store.RingCommand[T]{ChildLen: true})

	if err != nil {
		return 0
	}

	return result.ChildLen
}
