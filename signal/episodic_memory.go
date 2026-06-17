package signal

import (
	"io"
	"strconv"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

/*
symbolEpisodicMemory stores scoped observations in a DMT trie and a capped replay window.
*/
type symbolEpisodicMemory[T any] struct {
	tree     *dmt.Tree
	events   []T
	capacity int
	origin   string
	role     string
	readHead int
	mu       sync.Mutex
}

func newSymbolEpisodicMemory[T any](
	origin string,
	role string,
	capacity int,
) *symbolEpisodicMemory[T] {
	if capacity < 4 {
		capacity = FeedRingCapacity()
	}

	tree, _ := dmt.NewTree("")

	return &symbolEpisodicMemory[T]{
		tree:     tree,
		events:   make([]T, 0, capacity),
		capacity: capacity,
		origin:   origin,
		role:     role,
	}
}

func (memory *symbolEpisodicMemory[T]) appendEvent(
	record T,
	timestamp uint64,
	sequence []byte,
) {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	memory.events = append(memory.events, record)

	if len(memory.events) > memory.capacity {
		overflow := len(memory.events) - memory.capacity
		memory.events = memory.events[overflow:]
	}

	if memory.readHead > len(memory.events) {
		memory.readHead = 0
	}

	if memory.tree == nil || len(sequence) == 0 || timestamp == 0 {
		return
	}

	_, _ = memory.tree.CommitToEpisodicBuffer(timestamp, sequence)
	memory.tree.TrainSensorySequence(sequence)
}

func (memory *symbolEpisodicMemory[T]) eventsSnapshot() []T {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	snapshot := make([]T, len(memory.events))
	copy(snapshot, memory.events)

	return snapshot
}

func (memory *symbolEpisodicMemory[T]) readNext(
	buffer []byte,
	scope string,
	encode func(T) ([]byte, error),
) (int, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	if memory.readHead >= len(memory.events) {
		return 0, io.EOF
	}

	record := memory.events[memory.readHead]
	memory.readHead++

	payload, err := encode(record)

	if err != nil {
		return 0, err
	}

	outbound := datura.Acquire(memory.origin, datura.Artifact_Type_json)
	outbound.WithRole(memory.role)
	outbound.WithScope(scope)
	outbound.WithPayload(payload)

	return outbound.Read(buffer)
}

func (memory *symbolEpisodicMemory[T]) resetReadHead() {
	memory.mu.Lock()
	defer memory.mu.Unlock()

	memory.readHead = 0
}

func tradeSensorySequence(record TradeRecord) []byte {
	sideToken := "b"

	if record.Side == "sell" {
		sideToken = "s"
	}

	return []byte("t_" + sideToken + "_" + strconvFormatFloat(record.Price))
}

func bookSensorySequence(record BookRecord) []byte {
	if len(record.Bids) == 0 || len(record.Asks) == 0 {
		return nil
	}

	spread := record.Asks[0].Price - record.Bids[0].Price

	return []byte("b_" + strconvFormatFloat(spread))
}

func tickerSensorySequence(record TickerRecord) []byte {
	return []byte("k_" + strconvFormatFloat(record.Last))
}

func marshalRecordJSON(record any) ([]byte, error) {
	return sonic.Marshal(record)
}

func strconvFormatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
