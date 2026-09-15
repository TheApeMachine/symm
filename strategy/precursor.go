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
}

func NewPrecursor() *Precursor {
	return &Precursor{}
}

func (p *Precursor) SetExcursion(excursion *tables.ExcursionRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.excursion = excursion
}

func (p *Precursor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if in == nil || p.err != nil {
		return nil
	}

	return func(yield func(unsafe.Pointer) bool) {
		var activeSeq []byte

		for arriving := range in {
			if arriving == nil {
				continue
			}

			token := cognition.ExtractToken(arriving)
			if len(token) == 0 {
				continue
			}

			activeSeq = append(activeSeq, token...)

			imp := (*grid.Impulse)(arriving)
			tick := imp.SeqIdx

			p.mu.RLock()
			excursion := p.excursion
			p.mu.RUnlock()

			if excursion == nil {
				cmd := &cognition.Command{
					Evaluate: &cognition.Question{
						Context: bytes.Clone(activeSeq),
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
						Context: bytes.Clone(activeSeq),
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
							Context: bytes.Clone(activeSeq),
							Class:   []byte(ActionExit),
						},
					}

					if !yield(unsafe.Pointer(cmd)) {
						return
					}
				}

				activeSeq = activeSeq[:0]
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
