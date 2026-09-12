/*
Package associative is the learner strand of the streaming Primitive algebra:
the grid that forms a space of quantities and regions, the context that frames
their temporal trajectory into a token sequence, and the agent that sequences
learning and recall against the cognition engine that is its memory.

Everything is a streaming Primitive over an unsafe.Pointer wire. Payloads are
plain data types with exported fields only and no methods. Learn, recall and
reset are intents of one agent command because they operate on the agent's own
sequencing of its context and its engine.
*/
package associative

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Learn observes one impulse into memory: its regions are folded into the
temporal context, and the resulting sequence is associated with the class the
learner chose. Without a class it observes the sensory context alone.
*/
type Learn struct {
	Impulse  grid.Impulse
	Class    []byte
	Feedback float64
	Graded   bool
}

/*
Recall asks what the agent associates with the sequence the impulse extends,
without writing anything into memory.
*/
type Recall struct {
	Impulse grid.Impulse
}

/*
Command discriminates one learner intent. Exactly one field is set; any other
shape is a failure recorded in Error and ends the stream.
*/
type Command struct {
	Learn  *Learn
	Recall *Recall
	Reset  *ResetSignal
}

/*
Result is one command's answer: the encoded context sequence it produced, and
for recall the evaluation the engine gave it.
*/
type Result struct {
	Sequence   []byte
	Evaluation cognition.Evaluation
	Tree       *iradix.Tree[[]byte]
}

/*
Agent owns one learner: the regions it is shown become the sequence it
recognises, and what it recognises is written into the cognition engine that
is its memory. It composes the engine primitive and owns its own learn and
recall sequencing through its temporal context.
*/
type Agent struct {
	err     error
	out     Result
	engine  core.Primitive
	context core.Primitive
}

/*
NewAgent owns an engine of its own, or continues one it is given.

A composition declares its agent before it has a memory to hand it, and an
agent with nothing behind it is a learner that has not learned yet — which is
where every learner starts.
*/
func NewAgent(engine ...core.Primitive) core.Primitive {
	held := cognition.NewEngine(cognition.Config{})

	if len(engine) > 0 && engine[0] != nil {
		held = engine[0]
	}

	return &Agent{engine: held, context: NewContext()}
}

/*
Next executes each arriving command and yields its result. An invalid command
is recorded in Error and ends the stream.
*/
func (op *Agent) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*Command)(arriving)
			result, err := op.execute(command)

			if err != nil {
				op.Error(err)
				return
			}

			op.out = result

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Agent) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	if err := op.context.Error(); err != nil {
		op.err = errors.Join(op.err, err)
	}

	if err := op.engine.Error(); err != nil {
		op.err = errors.Join(op.err, err)
	}

	return op.err
}

/*
execute dispatches one command to its intent and returns its result.
*/
func (op *Agent) execute(command *Command) (Result, error) {
	intents := 0

	if command.Learn != nil {
		intents++
	}

	if command.Recall != nil {
		intents++
	}

	if command.Reset != nil {
		intents++
	}

	if intents != 1 {
		return Result{}, errShape("agent command must set exactly one intent")
	}

	if command.Learn != nil {
		return op.learn(*command.Learn)
	}

	if command.Recall != nil {
		return op.recall(command.Recall.Impulse)
	}

	return Result{}, op.reset()
}

/*
learn folds the impulse into the temporal context and observes the resulting
sequence into the engine. An impulse that produces no sequence yet has nothing
to observe: its result is the empty sequence, which is where every learner
starts.
*/
func (op *Agent) learn(intent Learn) (Result, error) {
	sequence, err := op.extend(intent.Impulse)

	if err != nil {
		return Result{}, err
	}

	if len(sequence) == 0 {
		return Result{}, nil
	}

	tree, err := ask(op.engine, &cognition.Command{Observe: &cognition.Association{
		Context:  sequence,
		Class:    intent.Class,
		Feedback: intent.Feedback,
		Graded:   intent.Graded,
	}})

	if err != nil {
		return Result{}, err
	}

	return Result{Sequence: sequence, Tree: tree.Tree}, nil
}

/*
recall folds the impulse into the temporal context and asks the engine what it
associates with the resulting sequence, without changing anything.
*/
func (op *Agent) recall(impulse grid.Impulse) (Result, error) {
	sequence, err := op.extend(impulse)

	if err != nil {
		return Result{}, err
	}

	result, err := ask(op.engine, &cognition.Command{Evaluate: &cognition.Question{Context: sequence}})

	if err != nil {
		return Result{}, err
	}

	return Result{Sequence: sequence, Evaluation: result.Evaluation}, nil
}

/* reset clears the agent's observation-local temporal history. */
func (op *Agent) reset() error {
	evaluation := transport.NewEvaluate(op.context)

	for range evaluation.Next(
		transport.NewOne(unsafe.Pointer(&ContextCommand{Reset: &ResetSignal{}})).Next(nil),
	) {
	}

	return evaluation.Error()
}

/*
extend folds one impulse into the temporal context and reads the encoded
sequence it produced.
*/
func (op *Agent) extend(impulse grid.Impulse) ([]byte, error) {
	evaluation := transport.NewEvaluate(op.context)
	var result ContextResult

	for out := range evaluation.Next(
		transport.NewOne(unsafe.Pointer(&ContextCommand{Encode: &impulse})).Next(nil),
	) {
		result = *(*ContextResult)(out)
	}

	return result.Sequence, evaluation.Error()
}

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
errShape builds an intent-shape failure.
*/
func errShape(detail string) error {
	return fmt.Errorf("%w: associative: %s", core.ErrShape, detail)
}
