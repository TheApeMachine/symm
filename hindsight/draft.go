package hindsight

import (
	"io"
	"strconv"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
StoreDrafts replaces run's working predictions with drafts. detect uses this
after it has stitched the run and scanned each symbol.
*/
func StoreDrafts(client *store.Client, run uint64, drafts []*data.Value) error {
	if err := clearDraftsForRun(client, run); err != nil {
		return err
	}

	for _, value := range drafts {
		if err := writeDraftTo(client, value); err != nil {
			return err
		}
	}

	return nil
}

func writeDraftTo(client *store.Client, value *data.Value) error {
	fields := metrics(value)

	for _, key := range []string{"a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick", "b_price", "c_price"} {
		if _, ok := fields[key]; !ok {
			return errnie.Err(errnie.UnprocessableContent, "hindsight: "+value.Prefix+" has no "+key, nil)
		}
	}

	if fields["a_tick"] < 1 {
		fields["a_tick"] = fields["b_tick"]
		fields["a_idx"] = fields["b_idx"]
	}

	ApplyExecutionLeadFields(fields)

	entryAsk := fields["b_price"]
	if v, ok := fields["b_ask"]; ok && v > 0 {
		entryAsk = v
	}

	exitBid := fields["c_price"]
	if v, ok := fields["c_bid"]; ok && v > 0 {
		exitBid = v
	}

	entryAt := fields["b_at"]
	exitAt := fields["c_at"]

	vector := []float64{
		fields["a_idx"], fields["a_tick"], fields["b_idx"], fields["b_tick"],
		fields["c_idx"], fields["c_tick"], fields["d_idx"], fields["d_tick"],
		fields["b_price"], fields["c_price"], entryAsk, exitBid,
		entryAt, exitAt,
		fields["entry_idx"], fields["entry_tick"], fields["exit_idx"], fields["exit_tick"],
		entryAsk, exitBid, entryAt, exitAt,
	}

	draft := data.NewValue("draft", value.Role, value.Scope)
	draft.Epoch = value.Epoch
	draft.Tick = value.Tick
	draft.Append(vector, excursionKeys...)

	_, err := io.Copy(client, draft)

	return err
}

func clearDraftsForRun(client *store.Client, run uint64) error {
	for _, class := range excursionClasses {
		for key := range client.ListKeys("draft/" + class + "/") {
			got, err := excursionEpoch(key)

			if err != nil || got != run {
				continue
			}

			if err := client.DeletePrefix(key); err != nil {
				return err
			}
		}
	}

	return nil
}

func excursionEpoch(key string) (uint64, error) {
	segments := strings.Split(key, "/")

	if len(segments) < 5 || len(segments[len(segments)-2]) != 19 {
		return 0, errnie.Err(errnie.UnprocessableContent, "[hindsight] excursion key "+key+" has no epoch segment", nil)
	}

	epoch, err := strconv.ParseUint(segments[len(segments)-2], 10, 64)

	if err != nil {
		return 0, errnie.Err(errnie.UnprocessableContent, "[hindsight] excursion key "+key+" has no epoch segment", err)
	}

	return epoch, nil
}
