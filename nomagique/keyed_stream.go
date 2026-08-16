package nomagique

import "sync"

/*
KeyedStreams owns one single-writer Stream per comparable key. The collection is
safe for concurrent routing across different keys; each individual key must
still have one ordered writer, normally a worker fed by a transport ring.

The key registry may allocate when a key is first observed. Established-key
transitions use the same reducer path as Stream.
*/
type KeyedStreams[Key comparable] struct {
	primitive Primitive
	initial   func(Key) Frame
	streams   sync.Map
}

/*
NewKeyedStreams creates a generic keyed reducer collection. initial may be nil,
in which case new keys start with an empty Frame.
*/
func NewKeyedStreams[Key comparable](
	primitive Primitive,
	initial func(Key) Frame,
) *KeyedStreams[Key] {
	return &KeyedStreams[Key]{
		primitive: primitive,
		initial:   initial,
	}
}

/*
Step routes one input to the stream selected by key.
*/
func (collection *KeyedStreams[Key]) Step(
	key Key,
	input Frame,
) (Frame, error) {
	stream, err := collection.stream(key)

	if err != nil {
		return Frame{}, err
	}

	return stream.Step(input)
}

/*
Project returns the last committed state for key.
*/
func (collection *KeyedStreams[Key]) Project(key Key) (Frame, bool) {
	stream, found := collection.load(key)

	if !found {
		return Frame{}, false
	}

	return stream.Project(), true
}

/*
Output returns the last successful output for key.
*/
func (collection *KeyedStreams[Key]) Output(key Key) (Frame, bool) {
	stream, found := collection.load(key)

	if !found {
		return Frame{}, false
	}

	return stream.Output(), true
}

/*
Error returns the last transition failure for key.
*/
func (collection *KeyedStreams[Key]) Error(key Key) (error, bool) {
	stream, found := collection.load(key)

	if !found {
		return nil, false
	}

	return stream.Error(), true
}

/*
Delete removes one keyed stream. The caller must ensure no worker is currently
processing that key.
*/
func (collection *KeyedStreams[Key]) Delete(key Key) {
	if collection == nil {
		return
	}

	collection.streams.Delete(key)
}

/*
Range visits committed keyed state snapshots. Returning false stops iteration.
*/
func (collection *KeyedStreams[Key]) Range(
	yield func(Key, Frame) bool,
) {
	if collection == nil || yield == nil {
		return
	}

	collection.streams.Range(func(storedKey any, storedValue any) bool {
		key, validKey := storedKey.(Key)
		stream, validStream := storedValue.(*Stream)

		if !validKey || !validStream {
			return true
		}

		return yield(key, stream.Project())
	})
}

func (collection *KeyedStreams[Key]) stream(key Key) (*Stream, error) {
	if collection == nil || collection.primitive == nil {
		return nil, primitiveError("keyed stream primitive is nil")
	}

	if stream, found := collection.load(key); found {
		return stream, nil
	}

	initial := Frame{}

	if collection.initial != nil {
		initial = collection.initial(key)
	}

	candidate := NewStream(collection.primitive, initial)
	stored, _ := collection.streams.LoadOrStore(key, candidate)
	stream, valid := stored.(*Stream)

	if !valid {
		return nil, primitiveError("keyed stream registry contains an invalid entry")
	}

	return stream, nil
}

func (collection *KeyedStreams[Key]) load(key Key) (*Stream, bool) {
	if collection == nil {
		return nil, false
	}

	stored, found := collection.streams.Load(key)

	if !found {
		return nil, false
	}

	stream, valid := stored.(*Stream)

	return stream, valid
}
