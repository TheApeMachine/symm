package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

var (
	ignitionHistoryNames = [historyFamilyCount]string{
		HistoryDeltas,
		HistoryRates,
		HistoryReturns,
	}
	ignitionHistoryCounts  [historyFamilyCount]nomagique.Symbol
	ignitionHistoryHeads   [historyFamilyCount]nomagique.Symbol
	ignitionHistorySamples [historyFamilyCount][MaxIgnitionHistory]nomagique.Symbol
)

func init() {
	for family := range historyFamilyCount {
		name := ignitionHistoryNames[family]

		ignitionHistoryCounts[family] = nomagique.MustIntern(
			fmt.Sprintf("history/%s/count", name),
		)

		ignitionHistoryHeads[family] = nomagique.MustIntern(
			fmt.Sprintf("history/%s/head", name),
		)

		for index := 0; index < MaxIgnitionHistory; index++ {
			if family == historyDeltas {
				ignitionHistorySamples[family][index] = nomagique.MustSampleSymbol(index)
				continue
			}

			ignitionHistorySamples[family][index] = nomagique.MustIntern(
				fmt.Sprintf("history/%s/sample/%d", name, index),
			)
		}
	}
}

/*
IgnitionHistorySample returns the symbol used by one retained history position.
HistoryMoves is a compatibility name for the returns family. HistoryPrecursors
selects that same return family because the positive-only precursor baseline is
derived from it without retaining a duplicate ring.
*/
func IgnitionHistorySample(name string, index int) (nomagique.Symbol, bool) {
	family, found := ignitionHistoryFamily(name)

	if !found || index < 0 || index >= MaxIgnitionHistory {
		return 0, false
	}

	return ignitionHistorySamples[family][index], true
}

/*
IgnitionHistoryCount returns the number of retained samples for one family.
*/
func IgnitionHistoryCount(state nomagique.Frame, name string) int {
	if name == HistoryPrecursors {
		return ignitionHistoryPositiveCount(state, historyReturns)
	}

	family, found := ignitionHistoryFamily(name)

	if !found {
		return 0
	}

	return int(number(state, ignitionHistoryCounts[family]))
}

func ignitionHistoryFamily(name string) (int, bool) {
	switch name {
	case HistoryDeltas:
		return historyDeltas, true
	case HistoryRates:
		return historyRates, true
	case HistoryMoves, HistoryReturns, HistoryPrecursors:
		return historyReturns, true
	default:
		return 0, false
	}
}

func ignitionHistoryPositiveCount(state nomagique.Frame, family int) int {
	count := int(number(state, ignitionHistoryCounts[family]))
	positive := 0

	for index := 0; index < count; index++ {
		value, found := state.Get(ignitionHistorySamples[family][index])

		if found && value > 0 {
			positive++
		}
	}

	return positive
}

func appendIgnitionHistory(
	state *nomagique.Frame,
	family int,
	capacity int,
	value float64,
	positiveOnly bool,
) error {
	if !finite(value) || positiveOnly && value <= 0 || !positiveOnly && value < 0 {
		return nil
	}

	count := int(number(*state, ignitionHistoryCounts[family]))
	head := int(number(*state, ignitionHistoryHeads[family]))

	if count < 0 || count > capacity || head < 0 || head >= capacity {
		return fmt.Errorf(
			"ignition: %s history is invalid for capacity %d",
			ignitionHistoryNames[family],
			capacity,
		)
	}

	slot := count

	if count >= capacity {
		slot = head
		head = (head + 1) % capacity
	} else {
		count++
	}

	state.Put(ignitionHistorySamples[family][slot], value)
	state.Put(ignitionHistoryCounts[family], float64(count))
	state.Put(ignitionHistoryHeads[family], float64(head))

	return nil
}

func ignitionHistoryMedian(
	state *nomagique.Frame,
	family int,
) (float64, bool, error) {
	count := int(number(*state, ignitionHistoryCounts[family]))

	if count == 0 {
		return 0, false, nil
	}

	if count < 0 || count > MaxIgnitionHistory {
		return 0, false, fmt.Errorf(
			"ignition: %s history count is invalid",
			ignitionHistoryNames[family],
		)
	}

	values := [MaxIgnitionHistory]float64{}

	for index := range count {
		value, found := state.Get(ignitionHistorySamples[family][index])

		if !found || !finite(value) {
			return 0, false, fmt.Errorf(
				"ignition: %s history sample %d is invalid",
				ignitionHistoryNames[family],
				index,
			)
		}

		values[index] = value
	}

	sortIgnitionValues(&values, count)

	middle := count / 2
	median := values[middle]

	if count%2 == 0 {
		median = (values[middle-1] + values[middle]) / 2
	}

	if math.IsNaN(median) || math.IsInf(median, 0) {
		return 0, false, fmt.Errorf(
			"ignition: %s history median is not finite",
			ignitionHistoryNames[family],
		)
	}

	return median, true, nil
}

func ignitionHistoryPositiveMedian(
	state *nomagique.Frame,
	family int,
) (float64, bool, error) {
	count := int(number(*state, ignitionHistoryCounts[family]))

	if count == 0 {
		return 0, false, nil
	}

	if count < 0 || count > MaxIgnitionHistory {
		return 0, false, fmt.Errorf(
			"ignition: %s history count is invalid",
			ignitionHistoryNames[family],
		)
	}

	values := [MaxIgnitionHistory]float64{}
	positive := 0

	for index := range count {
		value, found := state.Get(ignitionHistorySamples[family][index])

		if !found || !finite(value) {
			return 0, false, fmt.Errorf(
				"ignition: %s history sample %d is invalid",
				ignitionHistoryNames[family],
				index,
			)
		}

		if value <= 0 {
			continue
		}

		values[positive] = value
		positive++
	}

	if positive == 0 {
		return 0, false, nil
	}

	sortIgnitionValues(&values, positive)
	middle := positive / 2
	median := values[middle]

	if positive%2 == 0 {
		median = (values[middle-1] + values[middle]) / 2
	}

	return median, true, nil
}

func sortIgnitionValues(values *[MaxIgnitionHistory]float64, count int) {
	for index := 1; index < count; index++ {
		value := values[index]
		position := index

		for position > 0 && values[position-1] > value {
			values[position] = values[position-1]
			position--
		}

		values[position] = value
	}
}
