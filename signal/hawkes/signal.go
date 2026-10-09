package hawkes

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

type Signal struct {
	*runtime.System
	models   map[string]*nmhawkes.Hawkes
	lastTime int64
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		models:   make(map[string]*nmhawkes.Hawkes),
		lastTime: time.Now().Unix(),
	}

	signal.System = runtime.NewSystem(ctx, "hawkes", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		return nil
	}

	if prior.Source != "spot:trade" {
		signal.Error(errnie.Err(
			errnie.NotAcceptable,
			"[hawkes] non-trade frame "+prior.Source+" reached a trade-only signal",
			nil,
		))

		return nil
	}

	for _, key := range []string{"price", "qty"} {
		found := false

		for entry := range prior.Read(key) {
			if entry != nil && entry.Metric != nil {
				found = true
				break
			}
		}

		if !found {
			signal.Error(errnie.Err(
				errnie.Validation, "[hawkes] trade frame missing "+key, nil,
			))
			return nil
		}
	}

	side := prior.Meta("side")
	var mark float64

	if side == "buy" {
		mark = 1.0
	}

	if side == "sell" {
		mark = -1.0
	}

	if side != "buy" && side != "sell" {
		errnie.Warn(signal.Name() + ": trade without an explicit aggressor side; dropping event")
		return nil
	}

	atSec := float64(prior.At.UnixNano()) * 1e-9

	model, exists := signal.models[prior.Label]

	if !exists {
		model = nmhawkes.NewHawkes()
		signal.models[prior.Label] = model
	}

	res, err := model.Step(mark, atSec)

	if err != nil {
		signal.Error(errnie.Err(
			errnie.Internal,
			"[hawkes] "+prior.Label+": step failed",
			err,
		))

		return nil
	}

	prior.From = time.Unix(0, int64(signal.lastTime*1e9))
	signal.lastTime = int64(atSec)

	return prior.Next(signal.Name(), map[string]float64{
		"event_count":                                res[0],
		"event_count:buy":                            res[1],
		"event_count:sell":                           res[2],
		"event_fraction:buy":                         res[3],
		"event_fraction:sell":                        res[4],
		"arrival_rate:buy":                           res[5],
		"arrival_rate:sell":                          res[6],
		"arrival_rate":                               res[7],
		"conditional_intensity:buy":                  res[8],
		"conditional_intensity:sell":                 res[9],
		"conditional_intensity":                      res[10],
		"background_rate:buy":                        res[11],
		"background_rate:sell":                       res[12],
		"background_rate":                            res[13],
		"excitation_intensity:buy":                   res[14],
		"excitation_intensity:sell":                  res[15],
		"excitation_fraction:buy":                    res[16],
		"excitation_fraction:sell":                   res[17],
		"excitation_amplitude:buy_from_buy":          res[18],
		"excitation_amplitude:buy_from_sell":         res[19],
		"excitation_amplitude:sell_from_buy":         res[20],
		"excitation_amplitude:sell_from_sell":        res[21],
		"excitation_decay":                           res[22],
		"excitation_decay:buy_from_buy":              res[23],
		"excitation_decay:buy_from_sell":             res[24],
		"excitation_decay:sell_from_buy":             res[25],
		"excitation_decay:sell_from_sell":            res[26],
		"excitation_timescale":                       res[27],
		"excitation_timescale:buy_from_buy":          res[28],
		"excitation_timescale:buy_from_sell":         res[29],
		"excitation_timescale:sell_from_buy":         res[30],
		"excitation_timescale:sell_from_sell":        res[31],
		"offspring:buy_from_buy":                     res[32],
		"offspring:buy_from_sell":                    res[33],
		"offspring:sell_from_buy":                    res[34],
		"offspring:sell_from_sell":                   res[35],
		"branching_spectral_radius":                  res[36],
		"expected_descendants_from_buy":              res[37],
		"expected_descendants_from_sell":             res[38],
		"log_likelihood:hawkes":                      res[39],
		"log_likelihood_per_event:hawkes":            res[40],
		"log_likelihood:poisson":                     res[41],
		"log_likelihood_gain_vs_poisson":             res[42],
		"log_likelihood_gain_per_event_vs_poisson":   res[43],
		"log_likelihood:self_only":                   res[44],
		"log_likelihood_gain_vs_self_only":           res[45],
		"log_likelihood_gain_per_event_vs_self_only": res[46],
		"compensator:buy":                            res[47],
		"compensator:sell":                           res[48],
		"count_innovation:buy":                       res[49],
		"count_innovation:sell":                      res[50],
		"standardized_innovation:buy":                res[51],
		"standardized_innovation:sell":               res[52],
		"excitation_mass:buy":                        res[53],
		"excitation_mass:sell":                       res[54],
		"excitation_share:buy":                       res[55],
		"excitation_share:sell":                      res[56],
		"excitation_share":                           res[57],
		"historical_path_distance":                   res[59],
		"historical_path_percentile":                 res[60],
	})
}
