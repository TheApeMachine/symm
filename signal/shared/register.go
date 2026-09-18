package shared

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

var SymbolLast = [][]string{
	{"ticker", "data", "symbol"},
	{"ticker", "data", "last"},
}

var SymbolLastTime = [][]string{
	{"ticker", "data", "symbol"},
	{"ticker", "data", "last"},
	{"ticker", "data", "timestamp"},
}

var Ticker = [][]string{
	{"ticker", "data", "symbol"},
	{"ticker", "data", "last"},
	{"ticker", "data", "bid"},
	{"ticker", "data", "ask"},
	{"ticker", "data", "bid_qty"},
	{"ticker", "data", "ask_qty"},
	{"ticker", "data", "timestamp"},
}

var Trade = [][]string{
	{"trade", "data", "symbol"},
	{"trade", "data", "side"},
	{"trade", "data", "price"},
	{"trade", "data", "qty"},
	{"trade", "data", "timestamp"},
}


func Register(
	grid *store.Grid[*geometry.Coordinate], conn *transport.Conn[*geometry.Coordinate],
	interests [][]string,
) {
	nomagique.NewNumber(
		core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			conn, core.Identify,
		),
		grid,
	).Next(sequence.NewValue(interests))
}
