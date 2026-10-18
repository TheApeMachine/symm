package strategy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"golang.org/x/sync/errgroup"
)

type Trainer struct {
	*runtime.System
	training *Training
	client   *store.Client
	price    *broker.Price
	desk     *broker.Desk
}

func NewTrainer(
	ctx context.Context,
	training *Training,
	client *store.Client,
	price *broker.Price,
	desk *broker.Desk,
) *Trainer {
	trainer := &Trainer{
		System:   runtime.NewSystem(ctx, "trainer"),
		training: training,
		client:   client,
		price:    price,
		desk:     desk,
	}

	return trainer
}

func (trainer *Trainer) Generate() (err error) {
	groups, ctx := errgroup.WithContext(trainer.Context())

	for _, prefix := range trainer.client.ListKeys("detections/up/") {
		groups.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			buf, err := trainer.client.Download(prefix)

			if err != nil {
				return trainer.Error(err)
			}

			value := data.From(buf)

			fields, err := data.DetectExcursionFields(value)

			if err != nil {
				return trainer.Error(err)
			}

			actions := []string{
				"enter.bin", "exit.bin",
			}

			for idx, span := range [][]uint64{{
				uint64(fields["a_tick"]), uint64(fields["b_tick"]),
			}, {
				uint64(fields["b_tick"]), uint64(fields["c_tick"]),
			}} {
				val := data.NewValue(
					"policy", "current", "",
				)

				val.Prefix = "policy/current"

				for i := span[0]; i <= span[1]; i++ {
					buf, err := trainer.client.Download(
						fmt.Sprintf("workspace/tick/aggregate/%d/%019d", value.Epoch, i),
					)

					if err != nil {
						trainer.Error(err)
						continue
					}

					tick := data.From(buf)
					out := trainer.training.Step(tick)

					for index, _ := range data.Indexed[[]float64](out, "train/regions/") {
						suffix := strings.TrimPrefix(index, "train/regions/")
						val.Prefix += "/" + suffix
					}
				}

				val.Prefix += fmt.Sprintf("/%s", actions[idx])
				io.Copy(trainer.client, val)
			}

			return nil
		})
	}

	if err := groups.Wait(); err != nil {
		err = errors.Join(err, trainer.Error(err))
	}

	return err
}
