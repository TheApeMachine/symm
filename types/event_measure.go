package types

import (
	"context"
	"runtime"

	"golang.org/x/sync/errgroup"
)

/*
EventMeasurer turns one ordered market event into zero or more measurements.
Callers use it with MeasureEventsParallel so each symbol keeps serial event
order while unrelated symbols may run concurrently.
*/
type EventMeasurer func(event Event) ([]*Measurement, error)

/*
SymbolRows is one symbol's rows in first-seen symbol order from the input batch.
*/
type SymbolRows[T any] struct {
	Symbol string
	Rows   []T
}

/*
ChunkEventsBySymbol groups a globally ordered event batch into per-symbol chunks.
Each chunk preserves the relative order events had in the input slice, which
callers must already have sorted with OrderEvents when cross-stream ordering
matters.
*/
func ChunkEventsBySymbol(events []Event) []EventChunk {
	if len(events) == 0 {
		return nil
	}

	chunks := make([]EventChunk, 0, 8)
	order := make([]string, 0, 8)
	bySymbol := map[string][]Event{}

	for _, event := range events {
		rows, seen := bySymbol[event.Symbol]

		if !seen {
			order = append(order, event.Symbol)
		}

		bySymbol[event.Symbol] = append(rows, event)
	}

	for _, symbol := range order {
		chunks = append(chunks, EventChunk{
			Symbol: symbol,
			Events: bySymbol[symbol],
		})
	}

	return chunks
}

/*
ChunkRowsBySymbol groups rows by symbol while preserving each symbol's input order.
*/
func ChunkRowsBySymbol[T any](rows []T, symbolOf func(T) string) []SymbolRows[T] {
	if len(rows) == 0 {
		return nil
	}

	order := make([]string, 0, 8)
	bySymbol := map[string][]T{}

	for _, row := range rows {
		symbol := symbolOf(row)
		group, seen := bySymbol[symbol]

		if !seen {
			order = append(order, symbol)
		}

		bySymbol[symbol] = append(group, row)
	}

	chunks := make([]SymbolRows[T], len(order))

	for index, symbol := range order {
		chunks[index] = SymbolRows[T]{
			Symbol: symbol,
			Rows:   bySymbol[symbol],
		}
	}

	return chunks
}

/*
parallelWorkers returns the worker count for symbol-parallel work bounded by
GOMAXPROCS and the number of distinct symbol groups.
*/
func parallelWorkers(groupCount int) int {
	workers := min(runtime.GOMAXPROCS(0), groupCount)

	if workers < 1 {
		workers = 1
	}

	return workers
}

/*
MeasureEventsParallel applies measure to each event while preserving per-symbol
order and processing distinct symbols concurrently up to GOMAXPROCS workers.
Returned measurements follow first-seen symbol order, not completion order.
Per-event errors are skipped so one malformed row cannot abort unrelated symbols.
*/
func MeasureEventsParallel(
	events []Event,
	measure EventMeasurer,
) ([]*Measurement, error) {
	chunks := ChunkEventsBySymbol(events)

	if len(chunks) == 0 {
		return nil, nil
	}

	if len(chunks) == 1 {
		return measureEventChunk(chunks[0].Events, measure), nil
	}

	results := make([][]*Measurement, len(chunks))
	group := errgroup.Group{}
	group.SetLimit(parallelWorkers(len(chunks)))

	for index, chunk := range chunks {
		group.Go(func() error {
			results[index] = measureEventChunk(chunk.Events, measure)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	total := 0

	for _, part := range results {
		total += len(part)
	}

	out := make([]*Measurement, 0, total)

	for _, part := range results {
		out = append(out, part...)
	}

	return out, nil
}

/*
measureEventChunk applies measure serially to one symbol's ordered events.
*/
func measureEventChunk(
	events []Event,
	measure EventMeasurer,
) []*Measurement {
	out := make([]*Measurement, 0, len(events))

	for _, event := range events {
		measurements, err := measure(event)

		if err != nil {
			continue
		}

		out = append(out, measurements...)
	}

	return out
}

/*
RunSymbolGroupsParallel runs each symbol group's rows serially while distinct
symbols execute concurrently up to GOMAXPROCS workers. The first returned error
from any group aborts the remaining work.
*/
func RunSymbolGroupsParallel[T any](
	groups []SymbolRows[T],
	run func(index int, rows []T) error,
) error {
	if len(groups) == 0 {
		return nil
	}

	if len(groups) == 1 {
		return run(0, groups[0].Rows)
	}

	group, groupCtx := errgroup.WithContext(context.Background())
	group.SetLimit(parallelWorkers(len(groups)))

	for index, symbolGroup := range groups {
		group.Go(func() error {
			select {
			case <-groupCtx.Done():
				return groupCtx.Err()
			default:
			}

			return run(index, symbolGroup.Rows)
		})
	}

	return group.Wait()
}
