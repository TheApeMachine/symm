package guard

import (
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
IndexWindow is a chronological slice into a measurement tape.
*/
type IndexWindow struct {
	TrainStart int
	TrainEnd   int
	TestStart  int
	TestEnd    int
}

/*
GenerateIndexWindows builds rolling train/test index pairs over row count.

Train and test sizes are fractions of total rows; step advances the window
by stepFraction of the tape. Works without timestamps; preserves order.
*/
func GenerateIndexWindows(
	rowCount int,
	trainFraction float64,
	testFraction float64,
	stepFraction float64,
) []IndexWindow {
	if rowCount < 4 {
		return nil
	}

	if trainFraction <= 0 || testFraction <= 0 || stepFraction <= 0 {
		return nil
	}

	trainSize := int(float64(rowCount) * trainFraction)
	testSize := int(float64(rowCount) * testFraction)
	stepSize := int(float64(rowCount) * stepFraction)

	if trainSize < 2 || testSize < 1 || stepSize < 1 {
		return nil
	}

	windows := make([]IndexWindow, 0)
	trainStart := 0

	for {
		trainEnd := trainStart + trainSize
		testStart := trainEnd
		testEnd := testStart + testSize

		if testEnd > rowCount {
			break
		}

		windows = append(windows, IndexWindow{
			TrainStart: trainStart,
			TrainEnd:   trainEnd,
			TestStart:  testStart,
			TestEnd:    testEnd,
		})

		trainStart += stepSize

		if trainStart+trainSize+testSize > rowCount {
			break
		}
	}

	return windows
}

/*
GenerateTimeWindows builds hourly (or custom duration) rolling windows when
measurements carry timestamps.
*/
func GenerateTimeWindows(
	rows []perspectives.Measurement,
	trainDuration time.Duration,
	testDuration time.Duration,
	stepDuration time.Duration,
) []IndexWindow {
	if len(rows) == 0 || trainDuration <= 0 || testDuration <= 0 || stepDuration <= 0 {
		return nil
	}

	if rows[0].At.IsZero() {
		return nil
	}

	start := rows[0].At
	end := rows[len(rows)-1].At

	if !end.After(start) {
		return nil
	}

	windows := make([]IndexWindow, 0)
	cursor := start

	for {
		trainEnd := cursor.Add(trainDuration)
		testEnd := trainEnd.Add(testDuration)

		if testEnd.After(end) {
			break
		}

		trainStartIndex := indexAtOrAfter(rows, cursor)
		trainEndIndex := indexAtOrAfter(rows, trainEnd)
		testStartIndex := trainEndIndex
		testEndIndex := indexAtOrAfter(rows, testEnd)

		if trainEndIndex-trainStartIndex < 2 || testEndIndex-testStartIndex < 1 {
			cursor = cursor.Add(stepDuration)

			if cursor.Add(trainDuration + testDuration).After(end) {
				break
			}

			continue
		}

		windows = append(windows, IndexWindow{
			TrainStart: trainStartIndex,
			TrainEnd:   trainEndIndex,
			TestStart:  testStartIndex,
			TestEnd:    testEndIndex,
		})

		cursor = cursor.Add(stepDuration)
	}

	return windows
}

func indexAtOrAfter(rows []perspectives.Measurement, at time.Time) int {
	for index, row := range rows {
		if row.At.IsZero() {
			continue
		}

		if !row.At.Before(at) {
			return index
		}
	}

	return len(rows)
}
