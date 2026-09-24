package statistic

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
)

/*
GroupSumServer adds up the present values sharing a label.
*/
type GroupSumServer struct {
	labels []string
	sums   []float64
}

func NewGroupSum() *GroupSumServer {
	return &GroupSumServer{}
}

func (server *GroupSumServer) Write(ctx context.Context, call GroupSum_write) error {
	values, err := call.Args().Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.group_sum: failed to read values", err))
	}

	present, err := call.Args().Present()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.group_sum: failed to read present", err))
	}

	labels, err := call.Args().Labels()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.group_sum: failed to read labels", err))
	}

	server.labels, server.sums = server.labels[:0], server.sums[:0]

	// Labels that did not arrive leave nothing to group by. That is not a
	// group with a sum of zero, and it is not every value in one group.
	if labels.Len() == 0 {
		return nil
	}

	if present.Len() != values.Len() || labels.Len() != values.Len() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"statistic.group_sum: %d values with %d presence flags and %d labels",
				values.Len(), present.Len(), labels.Len(),
			),
			nil,
		))
	}

	groups := make(map[string]int)

	for element := range values.Len() {
		if !present.At(element) {
			continue
		}

		label, err := labels.At(element)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "statistic.group_sum: failed to read a label", err))
		}

		group, known := groups[label]

		if !known {
			group = len(server.labels)
			groups[label] = group
			server.labels = append(server.labels, label)
			server.sums = append(server.sums, 0)
		}

		server.sums[group] += values.At(element)
	}

	return nil
}

func (server *GroupSumServer) Done(ctx context.Context, call GroupSum_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.group_sum: failed to allocate results", err))
	}

	labels, err := results.NewLabels(int32(len(server.labels)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.group_sum: failed to allocate labels", err))
	}

	sums, err := results.NewSums(int32(len(server.sums)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.group_sum: failed to allocate sums", err))
	}

	for group, label := range server.labels {
		if err := labels.Set(group, label); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "statistic.group_sum: failed to set a label", err))
		}

		sums.Set(group, server.sums[group])
	}

	server.labels, server.sums = server.labels[:0], server.sums[:0]
	return nil
}
