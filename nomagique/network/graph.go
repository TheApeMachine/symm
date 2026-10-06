/*
Package network provides a lock-free graph Primitive backed by skip lists.

Everything is a core.Primitive over the unsafe.Pointer wire. The graph receives
**data.Adapter commands and publishes results on the same adapter. Domain
meaning lives in the caller's payloads; the graph knows only identity,
direction, weight, and traversal.

Command (text "op"):

	set_node    number "id"; text "data"
	set_edge    number "from","to","weight"; text "data"
	get_node    number "id"  → number "found"; text "data"
	outgoing    number "id"  → number "count"; number "edge.<i>.to",
	            "edge.<i>.weight"; text "edge.<i>.data"
	range_nodes number "from","to" → number "count"; number "node.<i>.id";
	            text "node.<i>.data"
	range_edges number "from","to" → number "count"; number "edge.<i>.from",
	            "edge.<i>.to","edge.<i>.weight"; text "edge.<i>.data"
	len         → number "len"

Identities travel as float64. Storage and point updates are lock-free.
*/
package network

import (
	"errors"
	"fmt"
	"iter"
	"strconv"
	"unsafe"

	"golang.design/x/lockfree"
	"golang.design/x/lockfree/lf"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Graph is a lock-free, mutable adjacency Primitive.
*/
type Graph struct {
	*core.PrimitiveError
	nodes *lf.SkipList[float64, string]
	edges *lf.SkipList[float64, [][3]any]
	roles data.Map[string]
	ids   data.Map[string]
	span  data.Map[string]
	edge  data.Map[string]
	literal data.Map[string]
	out   data.Map[float64]
	text  data.Map[string]
}

/*
NewGraph builds an empty graph Primitive. less orders identities; a nil less
is a shape failure and every stream yields nothing.
*/
func NewGraph(less lockfree.Less[float64]) core.Primitive {
	op := &Graph{
		PrimitiveError: core.NewPrimitiveError(),
		roles:          data.NewLiteral("op"),
		ids:            data.NewMap("id", "id"),
		span:           data.NewMap("from", "from", "to", "to"),
		edge:           data.NewMap("from", "from", "to", "to", "weight", "weight"),
		literal:        data.NewLiteral("data"),
		out:            data.NewOutputMap(),
		text:           data.NewTextMap(),
	}

	if less == nil {
		op.Error(fmt.Errorf("%w: network: graph requires an ordering function", core.ErrShape))
		return op
	}

	op.nodes = lf.NewSkipList[float64, string](less)
	op.edges = lf.NewSkipList[float64, [][3]any](less)

	return op
}

/*
Next receives **data.Adapter commands and yields the same adapter with results
published.
*/
func (op *Graph) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.Error() != nil {
		return func(func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var roles data.Map[string]

			for pointer := range adapter.Next(data.NewValue(op.roles)) {
				roles = *(*data.Map[string])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			intent, held := roles.Values["op"]

			if !held {
				op.Error(core.ErrNotHeld)
				return
			}

			clear(op.out.Values)
			clear(op.text.Values)

			switch intent {
			case "set_node":
				var numbers data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.ids)) {
					numbers = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				id, idOK := numbers.Values["id"]

				if !idOK {
					op.Error(core.ErrNotHeld)
					return
				}

				payload := ""
				var texts data.Map[string]

				for pointer := range adapter.Next(data.NewValue(op.literal)) {
					texts = *(*data.Map[string])(pointer)
				}

				if err := adapter.Error(); err != nil {
					if !errors.Is(err, core.ErrNotHeld) {
						op.Error(err)
						return
					}

					adapter.PrimitiveError = core.NewPrimitiveError()
				} else {
					payload = texts.Values["data"]
				}

				op.nodes.Set(id, payload)
				op.out.Values["len"] = float64(op.nodes.Len())

			case "set_edge":
				var numbers data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.edge)) {
					numbers = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				from, fromOK := numbers.Values["from"]
				to, toOK := numbers.Values["to"]
				weight, weightOK := numbers.Values["weight"]

				if !fromOK || !toOK || !weightOK {
					op.Error(core.ErrNotHeld)
					return
				}

				payload := ""
				var texts data.Map[string]

				for pointer := range adapter.Next(data.NewValue(op.literal)) {
					texts = *(*data.Map[string])(pointer)
				}

				if err := adapter.Error(); err != nil {
					if !errors.Is(err, core.ErrNotHeld) {
						op.Error(err)
						return
					}

					adapter.PrimitiveError = core.NewPrimitiveError()
				} else {
					payload = texts.Values["data"]
				}

				current, _ := op.edges.Get(from)
				replaced := false

				for index := range current {
					if current[index][0].(float64) == to {
						current[index] = [3]any{to, weight, payload}
						replaced = true
						break
					}
				}

				if !replaced {
					current = append(current, [3]any{to, weight, payload})
				}

				op.edges.Set(from, current)
				op.nodes.Set(from, "")
				op.nodes.Set(to, "")
				op.out.Values["len"] = float64(op.nodes.Len())

			case "get_node":
				var numbers data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.ids)) {
					numbers = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				id, idOK := numbers.Values["id"]

				if !idOK {
					op.Error(core.ErrNotHeld)
					return
				}

				payload, found := op.nodes.Get(id)

				if found {
					op.out.Values["found"] = 1
					op.text.Values["data"] = payload
				} else {
					op.out.Values["found"] = 0
				}

			case "outgoing":
				var numbers data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.ids)) {
					numbers = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				id, idOK := numbers.Values["id"]

				if !idOK {
					op.Error(core.ErrNotHeld)
					return
				}

				current, _ := op.edges.Get(id)
				op.out.Values["count"] = float64(len(current))

				for index, edge := range current {
					prefix := "edge." + strconv.Itoa(index)
					op.out.Values[prefix+".to"] = edge[0].(float64)
					op.out.Values[prefix+".weight"] = edge[1].(float64)
					op.text.Values[prefix+".data"] = edge[2].(string)
				}

			case "range_nodes":
				var numbers data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.span)) {
					numbers = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				from, fromOK := numbers.Values["from"]
				to, toOK := numbers.Values["to"]

				if !fromOK || !toOK {
					op.Error(core.ErrNotHeld)
					return
				}

				index := 0
				op.nodes.Range(from, to, func(id float64, payload string) {
					prefix := "node." + strconv.Itoa(index)
					op.out.Values[prefix+".id"] = id
					op.text.Values[prefix+".data"] = payload
					index++
				})
				op.out.Values["count"] = float64(index)

			case "range_edges":
				var numbers data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.span)) {
					numbers = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				from, fromOK := numbers.Values["from"]
				to, toOK := numbers.Values["to"]

				if !fromOK || !toOK {
					op.Error(core.ErrNotHeld)
					return
				}

				index := 0
				op.edges.Range(from, to, func(id float64, edges [][3]any) {
					for _, edge := range edges {
						prefix := "edge." + strconv.Itoa(index)
						op.out.Values[prefix+".from"] = id
						op.out.Values[prefix+".to"] = edge[0].(float64)
						op.out.Values[prefix+".weight"] = edge[1].(float64)
						op.text.Values[prefix+".data"] = edge[2].(string)
						index++
					}
				})
				op.out.Values["count"] = float64(index)

			case "len":
				op.out.Values["len"] = float64(op.nodes.Len())

			default:
				op.Error(fmt.Errorf("%w: network: unknown graph op %q", core.ErrShape, intent))
				return
			}

			for range adapter.Next(data.NewValue(op.out)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if len(op.text.Values) > 0 {
				for range adapter.Next(data.NewValue(op.text)) {
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
