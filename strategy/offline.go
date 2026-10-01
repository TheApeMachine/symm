package strategy

import (
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
DetectExcursions runs the same live Detector over a stored quote tape
(after-the-fact). Measurements must carry bid/ask (ticker or equivalent);
SeqIdx is the sync marker. Fees come from detector.price — never invented.
When detector.price is configured with an isolated Book, raw Level 3 mutations
are applied directly before evaluating fills.
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
		if row == nil || row.SeqIdx <= 0 || row.Label == "" {
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

	var isolatedBook *broker.Book

	if detector.price != nil && detector.price.Books != nil {
		if book, ok := detector.price.Books.(*broker.Book); ok {
			isolatedBook = book
		}
	}

	out := make([]tables.ExcursionRecord, 0)

	for _, row := range ordered {
		if isolatedBook != nil {
			isolatedBook.ApplyMeasurement(row)
		}

		if _, ok := quoteFrom(row); !ok {
			continue
		}

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

/*
ReconcileExcursions reconciles canonical hindsight excursions against provisional
live excursions. Canonical excursions supersede any provisional fragments that
overlap their [PrecursorStartTick, PostEndTick] interval. Superseded fragments
are discarded so they never teach false exits or chop.
Non-overlapping valid provisional live excursions are preserved.
*/
func ReconcileExcursions(
	canonical []tables.ExcursionRecord,
	provisional []tables.ExcursionRecord,
) ([]tables.ExcursionRecord, map[string]bool) {
	persistIDs := make(map[string]bool)
	provByID := make(map[string]tables.ExcursionRecord, len(provisional))

	for _, p := range provisional {
		provByID[p.ID] = p
	}

	superseded := make(map[string]bool)

	for _, c := range canonical {
		cStart := c.PrecursorStartTick
		cEnd := c.PostEndTick

		if c.ExitTick > cEnd {
			cEnd = c.ExitTick
		}

		for _, p := range provisional {
			if p.Symbol != c.Symbol {
				continue
			}

			pStart := p.PrecursorStartTick
			pEnd := p.PostEndTick

			if p.ExitTick > pEnd {
				pEnd = p.ExitTick
			}

			overlaps := pStart <= cEnd && cStart <= pEnd

			if overlaps && p.ID != c.ID {
				superseded[p.ID] = true
			}
		}

		if _, exists := provByID[c.ID]; !exists {
			persistIDs[c.ID] = true
		}
	}

	kept := make([]tables.ExcursionRecord, 0, len(canonical)+len(provisional))
	kept = append(kept, canonical...)

	for _, p := range provisional {
		if superseded[p.ID] {
			continue
		}

		alreadyIn := false

		for _, c := range canonical {
			if c.ID == p.ID {
				alreadyIn = true
				break
			}
		}

		if !alreadyIn {
			kept = append(kept, p)
		}
	}

	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Symbol != kept[j].Symbol {
			return kept[i].Symbol < kept[j].Symbol
		}

		return kept[i].AnchorTick < kept[j].AnchorTick
	})

	return kept, persistIDs
}

