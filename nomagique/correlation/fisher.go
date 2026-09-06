package correlation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewFisher reports the Fisher-z normal approximation for {correlation,support}.
// Optional search_count supplies Bonferroni multiplication. A finite-sample HY
// overlap count is NOT thereby made an independent sample size. The caller owns
// that statistical assumption. Undefined inputs yield defined=false and NaN,
// never a stale p-value. At +/-1 the extended-real limit yields p=0, not p=1.
func NewFisher() core.Primitive {
	undefined := equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0)))
	valid := equation.NewAll(
		transport.NewPipe(store.NewGet("correlation"), logic.NewFinite()),
		transport.NewPipe(store.NewGet("support"), logic.NewFinite()),
		equation.NewGreater[float64](store.NewGet("support"), store.NewConstant(core.From(3.0))),
		equation.NewLessEqual[float64](
			transport.NewPipe(store.NewGet("correlation"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
			store.NewConstant(core.From(1.0))))
	return transport.NewMap(logic.NewGate(
		equation.NewAll(store.NewHas("correlation"), store.NewHas("support")),
		transport.NewPipe(
			store.NewRecord(transport.NewPipe(), transport.NewPipe(valid, store.NewKey("defined"))),
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(logic.NewGate(store.NewGet("defined"), equation.NewFisher(), undefined), store.NewKey("p_value")),
				transport.NewPipe(logic.NewGate(store.NewGet("defined"),
					equation.NewProduct[float64](
						transport.NewPipe(store.NewGet("correlation"), calculus.NewAtanh(transport.NewIO(core.From(0.0)))),
						transport.NewPipe(equation.NewDifference[float64](store.NewGet("support"), store.NewConstant(core.From(3.0))),
							calculus.NewSqrt(transport.NewIO(core.From(0.0))))), undefined), store.NewKey("z")),
				transport.NewPipe(logic.NewGate(store.NewGet("defined"),
					equation.NewRatio[float64](store.NewConstant(core.From(1.0)),
						transport.NewPipe(equation.NewDifference[float64](store.NewGet("support"), store.NewConstant(core.From(3.0))),
							calculus.NewSqrt(transport.NewIO(core.From(0.0))))), undefined), store.NewKey("standard_error"))),
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(logic.NewGate(store.NewHas("search_count"),
					equation.NewAll(
						transport.NewPipe(store.NewGet("search_count"), logic.NewFinite()),
						equation.NewLessEqual[float64](store.NewConstant(core.From(1.0)), store.NewGet("search_count"))),
					store.NewConstant(core.From(false))), store.NewKey("has_search"))),
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(logic.NewGate(equation.NewAll(store.NewGet("defined"), store.NewGet("has_search")),
					transport.NewPipe(
						equation.NewProduct[float64](store.NewGet("p_value"), store.NewGet("search_count")),
						calculus.NewMinimum(transport.NewIO(core.From(1.0)))), undefined), store.NewKey("search_adjusted_p_value"))),
		), logic.NewReject(core.ErrShape)))
}
