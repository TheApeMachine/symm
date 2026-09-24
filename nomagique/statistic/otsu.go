package statistic

import (
	"context"
	"fmt"
	"slices"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
OtsuServer splits labelled values into a strong class and the rest where the
between-class variance peaks.
*/
type OtsuServer struct {
	hot       []string
	out       []byte
	threshold float64
}

func NewOtsu() *OtsuServer {
	return &OtsuServer{}
}

func (server *OtsuServer) Write(ctx context.Context, call Otsu_write) error {
	values, err := call.Args().Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.otsu: failed to read values", err))
	}

	labels, err := call.Args().Labels()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.otsu: failed to read labels", err))
	}

	if labels.Len() != values.Len() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("statistic.otsu: %d values with %d labels", values.Len(), labels.Len()),
			nil,
		))
	}

	candidates := make([]int, 0, values.Len())

	for element := range values.Len() {
		if values.At(element) > 0 {
			candidates = append(candidates, element)
		}
	}

	slices.SortStableFunc(candidates, func(left, right int) int {
		if values.At(left) > values.At(right) {
			return -1
		}

		if values.At(left) < values.At(right) {
			return 1
		}

		return 0
	})

	total := 0.0

	for _, element := range candidates {
		total += values.At(element)
	}

	cutoff := len(candidates)
	best, leading := 0.0, 0.0

	for split := 1; split < len(candidates); split++ {
		leading += values.At(candidates[split-1])
		difference := leading/float64(split) - (total-leading)/float64(len(candidates)-split)
		variance := float64(split*(len(candidates)-split)) * difference * difference

		if variance > best {
			best = variance
			cutoff = split
		}
	}

	server.hot = server.hot[:0]
	server.threshold = 0

	for _, element := range candidates[:cutoff] {
		label, err := labels.At(element)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "statistic.otsu: failed to read a label", err))
		}

		server.hot = append(server.hot, label)
		server.threshold = values.At(element)
	}

	slices.Sort(server.hot)

	encoded, err := sonic.Marshal(server.hot)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.otsu: failed to encode out", err))
	}

	server.out = encoded
	return nil
}

func (server *OtsuServer) Done(ctx context.Context, call Otsu_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.otsu: failed to allocate results", err))
	}

	hot, err := results.NewHot(int32(len(server.hot)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.otsu: failed to allocate hot", err))
	}

	for position, label := range server.hot {
		if err := hot.Set(position, label); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "statistic.otsu: failed to set a label", err))
		}
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.otsu: failed to set out", err))
	}

	results.SetThreshold(server.threshold)
	server.hot, server.out, server.threshold = server.hot[:0], nil, 0
	return nil
}
