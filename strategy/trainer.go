package strategy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	goruntime "runtime"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
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
	groups.SetLimit(goruntime.GOMAXPROCS(0))

	for prefix := range trainer.client.ListKeys("detections/up/") {
		groups.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			buf, err := object(trainer.Context(), trainer.client, prefix)

			if err != nil {
				return trainer.Error(err)
			}

			value := data.From(buf)

			fields, err := detectFields(value)

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
				step := trainer.training.fork()
				var sentence []string

				for i := span[0]; i <= span[1]; i++ {
					buf, err := object(trainer.Context(), trainer.client, fmt.Sprintf("workspace/tick/aggregate/%d/%019d", value.Epoch, i))

					if err != nil {
						trainer.Error(err)
						continue
					}

					tick := data.From(buf)
					out := step.Step(tick)

					if next := regionSentence(out, value.Scope); len(next) > 0 {
						sentence = next
					}
				}

				if len(sentence) == 0 {
					continue
				}

				val := data.NewValue("policy", "current", "")
				val.Prefix = "policy/current/" + strings.Join(sentence, "/") + "/" + actions[idx]

				if _, err := io.Copy(trainer.client, val); err != nil {
					return err
				}
			}

			return nil
		})
	}

	if waitErr := groups.Wait(); waitErr != nil {
		return errors.Join(waitErr, trainer.Error(waitErr))
	}

	return nil
}

/*
regionSentence is the last tick's region path for symbol: the Rxx labels
under train/regions/, without the symbol.
*/
func regionSentence(value *data.Value, symbol string) []string {
	if value == nil {
		return nil
	}

	var sentence []string

	for _, index := range value.Index {
		if !strings.Contains(index, "train/regions/") {
			continue
		}

		tokens := sentenceTokens(index, symbol)

		if len(tokens) == 0 {
			continue
		}

		if len(tokens) >= len(sentence) {
			sentence = tokens
		}
	}

	return sentence
}

func sentenceTokens(index, symbol string) []string {
	parts := strings.Split(index, "/")
	end := len(parts)
	start := end

	for start > 0 && isRegion(parts[start-1]) {
		start--
	}

	if start == end {
		return nil
	}

	if symbol == "" {
		return append([]string(nil), parts[start:end]...)
	}

	ownerStart := -1

	for i := start - 1; i > 0; i-- {
		if parts[i] == "regions" && parts[i-1] == "train" {
			ownerStart = i + 1
			break
		}
	}

	if ownerStart < 0 || strings.Join(parts[ownerStart:start], "/") != symbol {
		return nil
	}

	return append([]string(nil), parts[start:end]...)
}

func isRegion(part string) bool {
	if len(part) != 3 || part[0] != 'R' {
		return false
	}

	return part[1] >= '0' && part[1] <= '9' && part[2] >= '0' && part[2] <= '9'
}

func object(ctx context.Context, client *store.Client, key string) ([]byte, error) {
	var buf bytes.Buffer

	if err := client.Stream(ctx, key, &buf); err != nil {
		return nil, err
	}

	if buf.Len() == 0 {
		return nil, errnie.Err(errnie.NotFound, "strategy: "+key+" has no object", nil)
	}

	return buf.Bytes(), nil
}

func detectFields(value *data.Value) (map[string]float64, error) {
	keys := []string{
		"a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick",
		"b_price", "c_price",
	}

	if value == nil {
		return nil, errnie.Err(errnie.Validation, "strategy: no excursion to read", nil)
	}

	fields := map[string]float64{}

	for i, index := range value.Index {
		var values []float64
		if err := sonic.Unmarshal(value.Layers[i], &values); err != nil {
			continue
		}
		labels := vectorLabels(index, len(values))
		for j, reading := range values {
			if j < len(labels) {
				fields[labels[j]] = reading
			}
		}
	}

	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return nil, errnie.Err(errnie.UnprocessableContent, "strategy: "+value.Prefix+" has no "+key, nil)
		}
	}

	return fields, nil
}

/*
vectorLabels is the field names at the end of a detection address.
Stored keys keep the object name, then ".bin/", then the names.
*/
func vectorLabels(index string, count int) []string {
	rest := index

	if _, after, ok := strings.Cut(index, ".bin/"); ok {
		rest = after
	}

	labels := make([]string, 0)

	for part := range strings.SplitSeq(rest, "/") {
		if part != "" {
			labels = append(labels, part)
		}
	}

	if len(labels) > count {
		return labels[len(labels)-count:]
	}

	return labels
}
