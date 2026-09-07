package data

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"sync"
)

/*
projectionGraphs owns reusable, stateless projection compositions. Pool borrowing
provides exclusive graph ownership without a global market barrier. Stateful
estimators are never pooled; failed projection graphs are discarded.
*/
type projectionGraphs struct{ quality, authority, readout core.Primitive }

var projectionPool = sync.Pool{New: func() any {
	return &projectionGraphs{
		quality: NewQuality(), authority: NewAuthority(),
		readout: NewReadout(NewAuthority(), transport.NewIO(), transport.NewIO(),
			store.NewConstant(core.From(1.0)), store.NewGet("discrete")),
	}
}}
