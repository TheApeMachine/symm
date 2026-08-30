package broker

import (
	"github.com/theapemachine/symm/types"
)

/*
PerspectiveReader is the structural surface PositionRisk needs from the shared
advisor Perspective store. It is declared here, not imported from the advisor
package, so the broker depends only on the Latest method shape — the advisor
implementation stays behind the semantic boundary and can be swapped without
pulling its whole package into the guardian path.
*/
type PerspectiveReader interface {
	Latest(types.PerspectiveKey) (types.Perspective, bool)
}

/*
positionEntryContext is the fixed entry snapshot a Position captures when its
lot opens: the five Perspectives whose entry-vs-current comparison feeds the
profit-stagnation continuation context. It is a fixed struct, not an unbounded
map, so memory is constant per open position.
*/
type positionEntryContext struct {
	liquidity         types.Perspective
	liquidityDynamics types.Perspective
	flow              types.Perspective
	orderDisposition  types.Perspective
	executionContext  types.Perspective
}

/*
captureEntryContext snapshots the current latest Perspectives for the five
keys that participate in continuation support. Missing Perspectives are
captured as their zero value (Count == 0), which the continuation check treats
as "unavailable" — never as a reason to suppress an exit.
*/
func captureEntryContext(reader PerspectiveReader, symbol string) positionEntryContext {
	context := positionEntryContext{}

	if reader == nil {
		return context
	}

	if liquidity, found := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindLiquidity}); found {
		context.liquidity = liquidity
	}

	if dynamics, found := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindLiquidityDynamics}); found {
		context.liquidityDynamics = dynamics
	}

	if flow, found := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindFlow}); found {
		context.flow = flow
	}

	if disposition, found := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindOrderDisposition}); found {
		context.orderDisposition = disposition
	}

	if execution, found := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindExecutionContext}); found {
		context.executionContext = execution
	}

	return context
}
