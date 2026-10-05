package cognition

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

type Trainer struct {
	*core.PrimitiveError
	store core.Primitive
}

func NewTrainer(store core.Primitive) *Trainer {
	return &Trainer{
		PrimitiveError: core.NewPrimitiveError(),
		store:          store,
	}
}

/*
Next ingests a sequence of sensory tokens associated with a target class,
decomposing it into suffix n-grams up to MaxBackoffOrder with surprisal-modulated plasticity.
*/
func (op *Trainer) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	for arriving := range in {
		if arriving == nil {
			op.Error(core.ErrShape)
			return
		}

		adapter := *(**data.Adapter)(arriving)

		if adapter == nil {
			op.Error(core.ErrShape)
			return
		}

		var values data.Map[float64]

		for pointer := range adapter.Next(data.NewValue(op.input)) {
			values = *(*data.Map[float64])(pointer)
		}

		if err := adapter.Error(); err != nil {
			op.Error(err)
			return
		}

		if len(sequence) == 0 {
			return Result{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"[nomagique.cognition.engine] sequence is required for training",
				nil,
		))
	}

	if len(class) == 0 {
		return Result{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"[nomagique.cognition.engine] class is required for training",
			nil,
		))
	}

	evalResult, evalErr := op.evaluate(sequence)

	if evalErr != nil {
		return Result{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"[nomagique.cognition.engine] failed to evaluate training sequence",
			evalErr,
		))
	}

	plasticity := math.Min(1.0, 0.1+(evalResult.Evaluation.Surprisal/4.0))
	effectiveFeedback := feedback * plasticity
	var lastResult Result
	var err error

	if len(sequence) >= 12 {
		offset := 0
		var frameOffsets []int

		for offset < len(sequence) {
			if offset+4 > len(sequence) {
				frameOffsets = nil
				break
			}

			count := binary.BigEndian.Uint32(sequence[offset : offset+4])

			if count == 0 {
				frameOffsets = nil
				break
			}

			frameSize := 4 + int(count)*8

			if offset+frameSize > len(sequence) {
				frameOffsets = nil
				break
			}

			frameOffsets = append(frameOffsets, offset)
			offset += frameSize
		}

		if offset == len(sequence) && len(frameOffsets) > 0 {
			maxOrder := op.cfg.MaxBackoffOrder

			for startIdx := 0; startIdx < len(frameOffsets); startIdx++ {
				limitIdx := min(len(frameOffsets), startIdx+maxOrder)

				for endIdx := startIdx + 1; endIdx <= limitIdx; endIdx++ {
					var subContext []byte

					if endIdx < len(frameOffsets) {
						subContext = sequence[frameOffsets[startIdx]:frameOffsets[endIdx]]
					}

					if endIdx >= len(frameOffsets) {
						subContext = sequence[frameOffsets[startIdx]:]
					}

					lastResult, err = op.observe(Association{
						Context:  subContext,
						Class:    class,
						Feedback: effectiveFeedback,
						Graded:   true,
					})

					if err != nil {
						return Result{}, errnie.Error(errnie.Err(
							errnie.Validation,
							"[nomagique.cognition.engine] failed to observe training sequence",
							err,
						))
					}
				}
			}

			return lastResult, nil
		}
	}

	if len(sequence)%8 == 0 && len(sequence) >= 8 {
		tokenCount := len(sequence) / 8
		maxOrder := op.cfg.MaxBackoffOrder

		for startIdx := 0; startIdx < tokenCount; startIdx++ {
			limitIdx := min(tokenCount, startIdx+maxOrder)

			for endIdx := startIdx + 1; endIdx <= limitIdx; endIdx++ {
				subContext := sequence[startIdx*8 : endIdx*8]
				lastResult, err = op.observe(Association{
					Context:  subContext,
					Class:    class,
					Feedback: effectiveFeedback,
					Graded:   true,
				})

				if err != nil {
					return Result{}, err
				}
			}
		}

		return lastResult, nil
	}

	delim := byte(0)
	hasDelim := false

	if bytes.IndexByte(sequence, 0) >= 0 {
		delim = 0
		hasDelim = true
	}

	if !hasDelim && bytes.IndexByte(sequence, '/') >= 0 {
		delim = '/'
		hasDelim = true
	}

	if !hasDelim && bytes.IndexByte(sequence, '_') >= 0 {
		delim = '_'
		hasDelim = true
	}

	if hasDelim {
		var tokenBounds [][]int
		startOffset := 0

		for currentOffset, charByte := range sequence {
			if charByte == delim {
				if currentOffset > startOffset {
					tokenBounds = append(tokenBounds, []int{startOffset, currentOffset})
				}

				startOffset = currentOffset + 1
			}
		}

		if len(sequence) > startOffset {
			tokenBounds = append(tokenBounds, []int{startOffset, len(sequence)})
		}

		if len(tokenBounds) > 0 {
			maxOrder := op.cfg.MaxBackoffOrder

			for startIdx := 0; startIdx < len(tokenBounds); startIdx++ {
				limitIdx := min(len(tokenBounds), startIdx+maxOrder)

				for endIdx := startIdx + 1; endIdx <= limitIdx; endIdx++ {
					subContext := sequence[tokenBounds[startIdx][0]:tokenBounds[endIdx-1][1]]
					lastResult, err = op.observe(Association{
						Context:  subContext,
						Class:    class,
						Feedback: effectiveFeedback,
						Graded:   true,
					})

					if err != nil {
						return Result{}, errnie.Error(errnie.Err(
							errnie.Validation,
							"[nomagique.cognition.engine] failed to observe training sequence",
							err,
						))
					}
				}
			}

			return lastResult, nil
		}
	}

	lastResult, err = op.observe(Association{
		Context:  sequence,
		Class:    class,
		Feedback: effectiveFeedback,
		Graded:   true,
	})

	if err != nil {
		return Result{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"[nomagique.cognition.engine] failed to observe training sequence",
			err,
		))
	}

	_, suffixes := backoffCandidates(sequence, op.cfg.MaxBackoffOrder)

	for _, suffix := range suffixes {
		lastResult, err = op.observe(Association{
			Context:  suffix,
			Class:    class,
			Feedback: effectiveFeedback,
			Graded:   true,
		})

		if err != nil {
			return Result{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"[nomagique.cognition.engine] failed to observe training sequence",
				err,
			))
		}
	}

	return lastResult, nil
}
