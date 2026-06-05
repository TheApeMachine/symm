package reasoning

import (
	"hash/fnv"
	"math"
	"strconv"
	"sync"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
thoughtKey encodes a reasoning forest for search deduplication without routing hot
candidate identity through YAML. It preserves structural identity: fields that make
two thoughts evaluate or serialize differently are emitted in deterministic order.
*/
type thoughtKey struct {
	buffer []byte
}

var thoughtKeyPool = sync.Pool{
	New: func() any {
		return &thoughtKey{buffer: make([]byte, 0, 512)}
	},
}

func newThoughtKey() *thoughtKey {
	key := thoughtKeyPool.Get().(*thoughtKey)
	key.buffer = key.buffer[:0]

	return key
}

func releaseThoughtKey(key *thoughtKey) {
	thoughtKeyPool.Put(key)
}

func keyOf(forest []perspectives.Thought) string {
	key := newThoughtKey()
	key.writeForest(forest)
	value := string(key.buffer)
	releaseThoughtKey(key)

	return value
}

func (key *thoughtKey) fnv64() uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write(key.buffer)

	return hasher.Sum64()
}

func (key *thoughtKey) writeForest(forest []perspectives.Thought) {
	key.writeByte('F')
	key.writeInt(len(forest))

	for index := range forest {
		key.writeThought(forest[index])
	}
}

func (key *thoughtKey) writeThought(thought perspectives.Thought) {
	key.writeByte('T')
	key.writePredicate(thought.When)
	key.writeAct(thought.Do)
	key.writeForest(thought.Then)
}

func (key *thoughtKey) writePredicate(predicate perspectives.Predicate) {
	key.writeByte('P')
	key.writePredicates('A', predicate.All)
	key.writePredicates('Y', predicate.Any)

	if predicate.Not != nil {
		key.writeByte('N')
		key.writePredicate(*predicate.Not)
	}

	if predicate.Not == nil {
		key.writeByte('n')
	}

	key.writeInt(int(predicate.Subject))
	key.writeString(string(predicate.Category))
	key.writeInt(int(predicate.Regime))
	key.writeInt(int(predicate.Lifecycle))
	key.writeInt(int(predicate.Unit))
	key.writeInt(predicate.Ago)
	key.writeInt(int(predicate.Op))
	key.writeFloat(predicate.Value)
	key.writeOperand(predicate.Versus)
}

func (key *thoughtKey) writePredicates(
	marker byte,
	predicates []perspectives.Predicate,
) {
	key.writeByte(marker)
	key.writeInt(len(predicates))

	for index := range predicates {
		key.writePredicate(predicates[index])
	}
}

func (key *thoughtKey) writeOperand(operand *perspectives.Operand) {
	if operand == nil {
		key.writeByte('o')

		return
	}

	key.writeByte('O')
	key.writeInt(int(operand.Subject))
	key.writeString(string(operand.Category))
	key.writeInt(int(operand.Unit))
	key.writeInt(operand.Ago)
}

func (key *thoughtKey) writeAct(act perspectives.Act) {
	key.writeByte('D')
	key.writeInt(int(act.Type))
	key.writeFloat(act.Offset)
}

func (key *thoughtKey) writeString(value string) {
	key.writeInt(len(value))
	key.writeByte(':')
	key.buffer = append(key.buffer, value...)
}

func (key *thoughtKey) writeFloat(value float64) {
	if value == 0 {
		key.writeByte('0')

		return
	}

	key.writeByte('f')
	key.buffer = strconv.AppendUint(key.buffer, math.Float64bits(value), 10)
	key.writeByte('|')
}

func (key *thoughtKey) writeInt(value int) {
	key.buffer = strconv.AppendInt(key.buffer, int64(value), 10)
	key.writeByte('|')
}

func (key *thoughtKey) writeByte(value byte) {
	key.buffer = append(key.buffer, value)
}
