package pumpdump

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// newQuantityTarget retains observed quantities under the supplied adaptive
// window policy. Its median is an explicit reduction; no dynamic Store mode or
// non-positive-target substitution survives the migration.
func newQuantityTarget() core.Primitive {
	history := store.NewRetained(core.From([]float64{}))
	context := store.NewRetained(nil)
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(transport.NewPipe(), store.NewKey("value")),
			transport.NewPipe(adaptive.NewWindow(), store.NewGet("capacity"), calculus.NewConvert[float64, int](), store.NewKey("capacity"))),
		context, store.NewGet("value"), collection.NewAppend[float64](history),
		collection.NewTail[float64](transport.NewApply(store.NewGet("capacity"), context)),
		history, transport.NewSpread[float64](), equation.NewMedian(),
	)
}
