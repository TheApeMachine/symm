package cognition

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Basin is one attractor basin address: the class and the context it follows.
*/
type Basin struct {
	Class   []byte
	Context []byte
}

/*
Sensory is one sensory transition address: the context whose suffix
transition is being named.
*/
type Sensory struct {
	Context []byte
}

/*
KeyCommand discriminates one key operation: build the basin key for a class
under a context, build the sensory key for a context, or parse a stored key
back into its class and context. Exactly one intent is set; any other shape
is a failure recorded in Error and ends the stream.
*/
type KeyCommand struct {
	Basin   *Basin
	Sensory *Sensory
	Parse   []byte
}

/*
KeyResult is one key command's answer: the built key, or the class and
context a parsed key names, and whether the parse named a basin at all.
*/
type KeyResult struct {
	Key     []byte
	Class   []byte
	Context []byte
	Valid   bool
}

/*
Key owns the store's key layout: b/<context>/<class> for attractor basins and
s/<context> for sensory transitions. It is the one owner of that layout
outside the engine itself.
*/
type Key struct {
	err error
	out KeyResult
}

/*
NewKey instantiates the key layout Primitive.
*/
func NewKey() core.Primitive {
	return &Key{}
}

/*
Next executes each arriving key command and yields its result.
*/
func (op *Key) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*KeyCommand)(arriving)
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
func (op *Key) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
execute dispatches one key command to its intent.
*/
func (op *Key) execute(command *KeyCommand) (KeyResult, error) {
	intents := 0

	if command.Basin != nil {
		intents++
	}

	if command.Sensory != nil {
		intents++
	}

	if command.Parse != nil {
		intents++
	}

	if intents != 1 {
		return KeyResult{}, fmt.Errorf(
			"%w: cognition: key command must set exactly one intent",
			core.ErrShape,
		)
	}

	if command.Basin != nil {
		return KeyResult{Key: makeBasinKey(command.Basin.Class, command.Basin.Context), Valid: true}, nil
	}

	if command.Sensory != nil {
		return KeyResult{Key: makeSensoryKey(command.Sensory.Context), Valid: true}, nil
	}

	class, context, valid := parseBasinKey(command.Parse)

	return KeyResult{Class: class, Context: context, Valid: valid}, nil
}

/*
makeBasinKey builds b/<context>/<class>.
*/
func makeBasinKey(class, context []byte) []byte {
	buf := make([]byte, 2+len(context)+1+len(class))
	buf[0] = 'b'
	buf[1] = '/'
	copy(buf[2:], context)
	buf[2+len(context)] = '/'
	copy(buf[3+len(context):], class)
	return buf
}

/*
makeSensoryKey builds s/<context>.
*/
func makeSensoryKey(context []byte) []byte {
	buf := make([]byte, 2+len(context))
	buf[0] = 's'
	buf[1] = '/'
	copy(buf[2:], context)
	return buf
}

/*
parseBasinKey reads class and context back out of a b/<context>/<class> key.
Suffix/prefix matching order: exact (4), prefix (2), suffix (1)
Prefix matches dominant features; suffix matches secondary features.
*/
func parseBasinKey(k []byte) ([]byte, []byte, bool) {
	if len(k) < 4 || k[0] != 'b' || k[1] != '/' {
		return nil, nil, false
	}

	rem := k[2:]
	idx := bytes.LastIndexByte(rem, '/')

	if idx <= 0 || idx == len(rem)-1 {
		return nil, nil, false
	}

	return rem[idx+1:], rem[:idx], true
}
