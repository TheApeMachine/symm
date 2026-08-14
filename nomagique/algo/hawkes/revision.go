package hawkes

import (
	"time"

	"github.com/theapemachine/nomagique/hawkes"
)

/*
revision counts genuinely new timestamps between ordered observation windows.
A rolling-window replacement counts its entering event once, while replaying
an unchanged stream leaves the model's refit schedule untouched.
*/
type revision struct {
	buy  []time.Time
	sell []time.Time
}

func (revision *revision) Observe(stream hawkes.ArrivalStream) int {
	buy := stream.BuyTimes()
	sell := stream.SellTimes()
	changed := revision.additions(revision.buy, buy) +
		revision.additions(revision.sell, sell)
	revision.buy = append(revision.buy[:0], buy...)
	revision.sell = append(revision.sell[:0], sell...)

	return changed
}

func (revision *revision) additions(
	previous []time.Time,
	current []time.Time,
) int {
	previousIndex := 0
	currentIndex := 0
	added := 0

	for previousIndex < len(previous) && currentIndex < len(current) {
		if previous[previousIndex].Equal(current[currentIndex]) {
			previousIndex++
			currentIndex++
			continue
		}

		if previous[previousIndex].Before(current[currentIndex]) {
			previousIndex++
			continue
		}

		added++
		currentIndex++
	}

	return added + len(current) - currentIndex
}
