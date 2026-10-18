package hindsight

import (
	"context"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
)

/*
includedRetrace is the deepest counter-move inside prices, as a fraction of
the move from the first print to the last. That is the dip a committed
detection kept inside one swing.
*/
func includedRetrace(prices []float64) float64 {
	if len(prices) < 2 || prices[0] <= 0 {
		return 0
	}

	start, end := prices[0], prices[len(prices)-1]
	swing := math.Abs(end-start) / start

	if swing <= 1e-9 {
		return 0
	}

	up := end >= start
	extreme := start
	worst := 0.0

	for _, price := range prices {
		var counter float64

		if up {
			if price > extreme {
				extreme = price
			}

			counter = (extreme - price) / start
		} else {
			if price < extreme {
				extreme = price
			}

			counter = (price - extreme) / start
		}

		if fraction := counter / swing; fraction > worst {
			worst = fraction
		}
	}

	if worst > 0.9 {
		return 0.9
	}

	return worst
}

func scanningDetector() *strategy.Detector {
	price := broker.NewPrice(context.Background(), nil, nil, nil, nil)
	cash := 200.0
	fee := 0.8

	if system.Cfg != nil && system.Cfg.Market != nil {
		if system.Cfg.Market.Balance > 0 {
			cash = float64(system.Cfg.Market.Balance)
		}

		if system.Cfg.Market.TakerFee > 0 {
			fee = system.Cfg.Market.TakerFee
		}
	}

	price.SetReferenceCash(decimal.NewFromFloat64(cash))
	price.SetDefaultFee(kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(fee)})

	return strategy.NewDetector(context.Background(), price)
}

/*
LearnedRetrace is the pullback the last commit taught the detector. The next
detect run reads this one object and starts from it.
*/
func LearnedRetrace(client *store.Client) float64 {
	value := storedOne(client, "draft/params/learned")

	if value == nil {
		return 0
	}

	return metrics(value)["retrace"]
}

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

	vector := []float64{
		fields["a_idx"], fields["a_tick"], fields["b_idx"], fields["b_tick"],
		fields["c_idx"], fields["c_tick"], fields["d_idx"], fields["d_tick"],
		fields["b_price"], fields["c_price"], fields["b_price"], fields["c_price"],
		0, 0,
		fields["b_idx"], fields["b_tick"], fields["c_idx"], fields["c_tick"],
		fields["b_price"], fields["c_price"], 0, 0,
	}

	draft := data.NewValue("draft", value.Role, value.Scope)
	draft.Epoch = value.Epoch
	draft.Tick = value.Tick
	draft.Append(vector, excursionKeys...)

	_, err := io.Copy(client, draft)

	return err
}

/*
excursionEpoch is the epoch segment of an excursion key. The symbol holds
slashes, so the epoch is the second segment from the end.
*/
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
