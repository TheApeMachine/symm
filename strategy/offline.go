package strategy

import (
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
DetectExcursions runs the same live Detector over a stored quote tape
(after-the-fact). Measurements must carry bid/ask (ticker or equivalent);
SeqIdx is the sync marker. Fees come from detector.price — never invented.
*/
func DetectExcursions(
	detector *Detector,
	tape []*data.Measurement[float64],
) ([]tables.ExcursionRecord, error) {
	if detector == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"offline: detector is required",
			nil,
		))
	}

	ordered := make([]*data.Measurement[float64], 0, len(tape))

	for _, row := range tape {
		if row == nil {
			continue
		}

		if _, ok := quoteFrom(row); !ok {
			continue
		}

		ordered = append(ordered, row)
	}

	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Label != ordered[right].Label {
			return ordered[left].Label < ordered[right].Label
		}

		return ordered[left].SeqIdx < ordered[right].SeqIdx
	})

	out := make([]tables.ExcursionRecord, 0)

	for _, row := range ordered {
		record, err := detector.Observe(row)

		if err != nil {
			return out, err
		}

		if record != nil {
			out = append(out, *record)
		}
	}

	return out, nil
}
