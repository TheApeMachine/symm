package learning

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPace composes empirical prior-error rank with the source's bounded log
// alpha controller. config yields rest,lower,upper,gain,band,window. These are
// explicit configuration, not newly chosen tuning constants. The calibrator
// owns history, the retained connection owns log alpha, and Mix owns movement.
func NewPace(config core.Primitive) core.Primitive {
	context := store.NewRetained(nil)
	memory := store.NewRetained(core.From(map[string]core.Primitive{}))
	calibrator := probability.NewCalibrator(
		collection.NewTail[float64](
			transport.NewApply(transport.NewPipe(
				store.NewGet("window"), calculus.NewConvert[float64, int]()), context),
		),
	)
	prepare := transport.NewPipe(
		store.NewRecord(config, transport.NewPipe(transport.NewPipe(), store.NewKey("error"))),
		context,
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("rest"), calculus.NewLog(transport.NewIO(core.From(0.0))), store.NewKey("log_rest")),
			transport.NewPipe(store.NewGet("lower"), calculus.NewLog(transport.NewIO(core.From(0.0))), store.NewKey("log_min")),
			transport.NewPipe(store.NewGet("upper"), calculus.NewLog(transport.NewIO(core.From(0.0))), store.NewKey("log_max")),
		),
	)
	seed := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(store.NewGet("log_rest"), store.NewKey("log_alpha")),
		transport.NewPipe(store.NewGet("rest"), store.NewKey("alpha")),
	)
	measure := transport.NewPipe(
		store.NewKV[string](memory),
		logic.NewGate(store.NewHas("log_alpha"), transport.NewPipe(), seed),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("error"), calibrator, store.NewKey("calibration"))),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewLessEqual[float64](
					store.NewGet("window"),
					transport.NewPipe(store.NewGet("calibration"), store.NewGet("prior_count")),
				),
				store.NewKey("ready"),
			),
			transport.NewPipe(
				equation.NewMinimum(
					store.NewGet("window"),
					equation.NewSum[float64](
						transport.NewPipe(store.NewGet("calibration"), store.NewGet("prior_count")),
						store.NewConstant(core.From(1.0)),
					),
				),
				store.NewKey("count"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				logic.NewGate(
					store.NewGet("ready"),
					transport.NewPipe(store.NewGet("calibration"), store.NewGet("value")),
					store.NewConstant(core.From(0.0)),
				),
				store.NewKey("rank"),
			),
		),
	)
	update := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				logic.NewGate(
					equation.NewLess[float64](store.NewGet("rank"), store.NewGet("band")),
					store.NewGet("log_max"),
					store.NewGet("log_rest"),
				),
				store.NewKey("target"),
			),
		),
		logic.NewGate(
			equation.NewGreater[float64](
				store.NewGet("rank"),
				equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("band")),
			),
			store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("log_min"), store.NewKey("target"))),
			transport.NewPipe(),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewBound(
					transport.NewPipe(
						store.NewRecord(
							transport.NewPipe(store.NewGet("log_alpha"), store.NewKey("left")),
							transport.NewPipe(store.NewGet("target"), store.NewKey("right")),
							transport.NewPipe(store.NewGet("gain"), store.NewKey("weight")),
						),
						equation.NewMix(),
					),
					store.NewGet("log_min"),
					store.NewGet("log_max"),
				),
				store.NewKey("log_alpha"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewBound(
					transport.NewPipe(store.NewGet("log_alpha"), calculus.NewExp(transport.NewIO(core.From(0.0)))),
					store.NewGet("lower"),
					store.NewGet("upper"),
				),
				store.NewKey("alpha"),
			),
		),
	)
	commit := transport.NewFan(
		transport.NewPipe(),
		transport.NewIO(
			transport.NewPipe(
				store.NewRecord(
					transport.NewPipe(store.NewGet("log_alpha"), store.NewKey("log_alpha")),
					transport.NewPipe(store.NewGet("alpha"), store.NewKey("alpha")),
				),
				memory,
				transport.NewDiscard(),
			),
			transport.NewPipe(),
		),
	)
	return transport.NewMap(
		transport.NewPipe(
			prepare,
			logic.NewGate(
				equation.NewAll(
					transport.NewPipe(store.NewGet("error"), logic.NewFinite()),
					transport.NewPipe(store.NewGet("rest"), logic.NewFinite()),
					transport.NewPipe(store.NewGet("lower"), logic.NewFinite()),
					transport.NewPipe(store.NewGet("upper"), logic.NewFinite()),
					transport.NewPipe(store.NewGet("gain"), logic.NewFinite()),
					transport.NewPipe(store.NewGet("band"), logic.NewFinite()),
					transport.NewPipe(store.NewGet("window"), logic.NewFinite()),
					equation.NewGreater[float64](store.NewGet("lower"), store.NewConstant(core.From(0.0))),
					equation.NewLessEqual[float64](store.NewGet("lower"), store.NewGet("rest")),
					equation.NewLessEqual[float64](store.NewGet("rest"), store.NewGet("upper")),
					equation.NewGreater[float64](store.NewGet("window"), store.NewConstant(core.From(0.0))),
					equation.NewEqual[float64](
						store.NewGet("window"),
						transport.NewPipe(store.NewGet("window"), calculus.NewFloor(transport.NewIO(core.From(0.0)))),
					),
				),
				transport.NewPipe(measure, logic.NewGate(store.NewGet("ready"), update, transport.NewPipe()), commit),
				logic.NewReject(core.ErrDomain),
			),
		),
	)
}
