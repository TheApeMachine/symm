package compiler

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/ui"
)

/*
bindings encodes the values that reached component ports in the evaluation
that just finished, one entry per edge into a component. A producer that said
nothing this evaluation contributes nothing, so a component keeps what it was
last shown rather than being handed an empty value. It returns nil when no
bound producer spoke.
*/
func (p *Program) bindings() ([]byte, error) {
	if p.Bindings == nil || len(p.Bindings.Bindings) == 0 {
		return nil, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	type arrival struct {
		entry BindingEntry
		value string
	}

	projection := &resultProjection{nodes: make(map[uint64]schema.Node)}
	arrivals := make([]arrival, 0, len(p.Bindings.Bindings))

	if p.published == nil {
		p.published = make([]string, len(p.Bindings.Bindings))
	}

	// A surface that just joined is owed what every port is showing, not only
	// what changed on this evaluation.
	told := make([]bool, len(p.Bindings.Bindings))

	if p.replay {
		p.replay = false

		for slot, encoded := range p.published {
			if encoded == "" {
				continue
			}

			told[slot] = true
			arrivals = append(arrivals, arrival{p.Bindings.Bindings[slot], encoded})
		}
	}

	for slot, entry := range p.Bindings.Bindings {
		index, known := p.NodeMap[entry.SourceNode]

		if !known {
			continue
		}

		field, declared := p.Nodes[index].Outputs[entry.SourcePort]

		if !declared {
			continue
		}

		result, found := p.results[entry.SourceNode]

		if !found || !result.IsValid() {
			continue
		}

		if field.InUnion && result.Uint16(capnp.DataOffset(field.DiscriminantOffset*2)) != field.DiscriminantValue {
			continue
		}

		value, err := projection.Field(result, field.SchemaField)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("compiler: project bound %s.%s", entry.SourceNode, entry.SourcePort),
				err,
			))
		}

		encoded, spoke, err := boundJSON(value)

		if err != nil {
			return nil, err
		}

		// A component already showing this value is not told it again.
		if !spoke || p.published[slot] == encoded {
			continue
		}

		p.published[slot] = encoded

		if told[slot] {
			arrivals = slices.DeleteFunc(arrivals, func(candidate arrival) bool {
				return candidate.entry == entry
			})
		}

		arrivals = append(arrivals, arrival{entry, encoded})
	}

	if len(arrivals) == 0 {
		return nil, nil
	}

	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Internal, "compiler: bindings message", err))
	}

	frame, err := ui.NewRootBindings(segment)

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Internal, "compiler: bindings root", err))
	}

	values, err := frame.NewValues(int32(len(arrivals)))

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Internal, "compiler: bindings list", err))
	}

	for index, arrived := range arrivals {
		bound := values.At(index)

		for _, err := range []error{
			bound.SetGraph(arrived.entry.TargetGraph),
			bound.SetComponent(arrived.entry.TargetComponent),
			bound.SetProp(arrived.entry.TargetProp),
			bound.SetValue(arrived.value),
		} {
			if err != nil {
				return nil, errnie.Error(errnie.Err(errnie.Internal, "compiler: set bound value", err))
			}
		}
	}

	encoded, err := message.Marshal()

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Internal, "compiler: marshal bindings", err))
	}

	return encoded, nil
}

/*
boundJSON is the JSON a component receives for a projected port value. Data a
producer emits is already a document when it is JSON, and is handed over as
that document rather than as its bytes. An empty Data or Text value is a
producer that said nothing.
*/
func boundJSON(value any) (string, bool, error) {
	switch typed := value.(type) {
	case nil:
		return "", false, nil
	case []byte:
		if len(typed) == 0 {
			return "", false, nil
		}

		if json.Valid(typed) {
			return string(typed), true, nil
		}
	case string:
		if typed == "" {
			return "", false, nil
		}
	case float64:
		// A reading that is not a finite number has no value to show.
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return "null", true, nil
		}
	}

	encoded, err := json.Marshal(value)

	if err != nil {
		return "", false, errnie.Error(errnie.Err(errnie.Internal, "compiler: encode bound value", err))
	}

	return string(encoded), true, nil
}

/*
Refresh has the next evaluation deliver every bound value last published, as
well as what changed — what a surface that just connected needs, even when
the producers behind it have gone quiet.
*/
func (p *Program) Refresh() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.replay = true
}
