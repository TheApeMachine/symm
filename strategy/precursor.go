package strategy

import (
	"bytes"
	"errors"
	"iter"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/*
Precursor is the streaming primitive between grid.Space and cognition.Engine.
It accumulates active region tokens into temporal sequences and translates
excursion ticks into supervised cognition.Association commands at AnchorTick and ExitTick.
When running without an excursion (live inference), it translates active sequences into cognition.Question commands.
*/
type Precursor struct {
	mu        sync.RWMutex
	err       error
	excursion *tables.ExcursionRecord
	activeSeq []byte
	maxSeqLen int
}

func NewPrecursor(maxSeqLen ...int) *Precursor {
	limit := 128
	if len(maxSeqLen) > 0 && maxSeqLen[0] > 0 {
		limit = maxSeqLen[0]
	}

	return &Precursor{
		maxSeqLen: limit,
	}
}

func (p *Precursor) SetExcursion(excursion *tables.ExcursionRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.excursion = excursion
	p.activeSeq = p.activeSeq[:0]
}

func (p *Precursor) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activeSeq = p.activeSeq[:0]
	p.excursion = nil
}

func (p *Precursor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if in == nil || p.err != nil {
		return nil
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			token := cognition.ExtractToken(arriving)
			if len(token) == 0 {
				continue
			}

			p.mu.Lock()
			p.activeSeq = append(p.activeSeq, token...)
			if p.maxSeqLen > 0 && len(p.activeSeq) > p.maxSeqLen {
				p.activeSeq = p.activeSeq[len(p.activeSeq)-p.maxSeqLen:]
			}
			context := bytes.Clone(p.activeSeq)
			excursion := p.excursion
			p.mu.Unlock()

			imp := (*grid.Impulse)(arriving)
			tick := imp.SeqIdx

			if excursion == nil {
				cmd := &cognition.Command{
					Evaluate: &cognition.Question{
						Context: context,
					},
				}

				if !yield(unsafe.Pointer(cmd)) {
					return
				}

				continue
			}

			if tick == excursion.AnchorTick {
				class := ActionWait

				if excursion.Direction == "upward" && excursion.ClearsFriction {
					class = ActionEnter
				}

				cmd := &cognition.Command{
					Observe: &cognition.Association{
						Context: context,
						Class:   []byte(class),
					},
				}

				if !yield(unsafe.Pointer(cmd)) {
					return
				}

				continue
			}

			if tick == excursion.ExitTick {
				if excursion.Direction == "upward" && excursion.ClearsFriction {
					cmd := &cognition.Command{
						Observe: &cognition.Association{
							Context: context,
							Class:   []byte(ActionExit),
						},
					}

					if !yield(unsafe.Pointer(cmd)) {
						return
					}
				}

				p.mu.Lock()
				p.activeSeq = p.activeSeq[:0]
				p.mu.Unlock()
				continue
			}
		}
	}
}

func (p *Precursor) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			p.err = errors.Join(p.err, err)
		}
	}

	return p.err
}
