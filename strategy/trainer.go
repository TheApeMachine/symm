package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

type Trainer struct {
	*runtime.System
	catalog *tables.Catalog
}

func NewTrainer(
	ctx context.Context,
	catalog *tables.Catalog,
) *Trainer {
	trainer := &Trainer{
		catalog: catalog,
	}

	trainer.System = runtime.NewSystem(ctx, "trainer")
	return trainer
}

func (trainer *Trainer) Generate() error {
	grid := store.NewGrid()
	trained := 0

	for excursion := range trainer.catalog.Excursions(trainer.Context()) {
		if excursion == nil {
			continue
		}

		class := excursion.Meta("type")

		if class == "" {
			class = excursion.Meta("class")
		}

		if class == "" {
			class = excursion.Meta("direction")
		}

		isUp := strings.EqualFold(class, excursionUp)
		fragments := [][]string{{"start_tick", "b_tick"}}
		actions := [][]byte{[]byte(actionNoop)}

		if isUp {
			fragments = [][]string{{"start_tick", "b_tick"}, {"b_tick", "c_tick"}}
			actions = [][]byte{[]byte(actionEnter), []byte(actionExit)}
		}

		tokens := make([][][]byte, len(fragments))

		for idx, fragment := range fragments {
			ticks := make(map[int64][]*data.Measurement)

			low, end := fragmentTicks(
				int64(data.Pull(excursion.Read(fragment[0])).Metric.Raw),
				int64(data.Pull(excursion.Read(fragment[1])).Metric.Raw),
			)

			if end < low {
				continue
			}

			for tape, err := range trainer.catalog.ExcursionTape(
				trainer.Context(),
				excursion.Epoch,
				excursion.Label,
				low,
				end,
			) {
				if err != nil {
					errnie.Error(errnie.Err(
						errnie.IO,
						"[training] tape read error",
						err,
					))

					continue
				}

				if tape != nil {
					ticks[tape.Tick] = append(ticks[tape.Tick], tape)
				}
			}

			for tick := low; tick <= end; tick++ {
				signals := ticks[tick]

				if len(signals) == 0 {
					continue
				}

				train := data.NewMeasurement(
					signals[0].Epoch,
					excursion.Label,
					"training",
					int64(data.Pull(excursion.Read("start_idx")).Metric.Raw),
					tick,
				)

				for _, signal := range signals {
					if signal.At.After(train.At) {
						train.At = signal.At
					}

					if !signal.From.IsZero() && (train.From.IsZero() || signal.From.Before(train.From)) {
						train.From = signal.From
					}
				}

				train.Peers(signals...)
				train.Write()

				token := grid.Observe(train)

				if len(tokens[idx]) > 0 && bytes.Equal(
					token, tokens[idx][len(tokens[idx])-1],
				) {
					continue
				}

				tokens[idx] = append(tokens[idx], token)
			}
		}

		for idx, action := range actions {
			if len(tokens[idx]) == 0 {
				continue
			}

			tokens[idx] = append(tokens[idx], action)

			trainer.catalog.PutBlob(
				trainer.Context(),
				string(bytes.Join(tokens[idx], []byte("/"))),
				trainer.excursionStats(excursion),
			)
		}

		trained++
	}

	errnie.Info(fmt.Sprintf("[training] Train finished: %d excursions processed into token paths", trained))
	return nil

}

/*
excursionStats is the blob Train stores under both token paths of one
excursion.
*/
func (trainer *Trainer) excursionStats(excursion *data.Measurement) []byte {
	stats := pathStats{}

	if b, c, err := tables.DetectionPrices(excursion); err == nil && b.Sign() > 0 {
		gain := c.Float64()/b.Float64() - 1
		stats.Gain = &gain
	}

	if !excursion.At.IsZero() && !excursion.From.IsZero() && excursion.At.After(excursion.From) {
		hold := excursion.At.Sub(excursion.From).Seconds()
		stats.HoldSeconds = &hold
	}

	body, err := json.Marshal(stats)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[training] failed to marshal path statistics",
			err,
		))

		return []byte("{}")
	}

	return body
}
