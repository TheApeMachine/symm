package compiler

import (
	"context"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* Step evaluates the same compiled graph when its capability is invoked by LMAX. */
func (p *Program) Step(ctx context.Context, call runtime.StageNode_step) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stageArrivals = nil
	frames := make([]nodeFrame, len(p.Nodes))
	defer func() {
		for _, frame := range frames {
			if frame.args.IsValid() {
				frame.args.Message().Release()
			}
		}
	}()
	entry, err := call.Args().Entry()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "program: entry", err))
	}

	if entry != "" {
		if err := p.seedEntry(frames, entry, call.Args()); err != nil {
			return err
		}
	}
	ready, err := p.seedBindings(frames, call.Args())

	if err != nil {
		return err
	}

	if !ready {
		return nil
	}
	if err := p.evaluate(ctx, frames); err != nil {
		return err
	}
	return p.exportStep(call)
}

/* seedEntry supplies raw observations or native text lists through authored inputs. */
func (p *Program) seedEntry(frames []nodeFrame, entry string, input runtime.StageNode_step_Params) error {
	index, field, err := p.entry(entry)

	if err != nil {
		return err
	}
	payload, err := input.Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "program: input payload", err))
	}
	args, err := p.arguments(frames, index)

	if err != nil {
		return err
	}

	switch field.Which {
	case schema.Type_Which_data:
		err = args.SetData(uint16(field.Offset), payload)
	case schema.Type_Which_list:
		if field.ElementWhich == schema.Type_Which_text {
			texts, err := input.Texts()

			if err != nil {
				return errnie.Error(err)
			}

			if err := args.SetPtr(uint16(field.Offset), texts.ToPtr()); err != nil {
				return errnie.Error(err)
			}
			frames[index].ready |= 1 << p.Nodes[index].Inputs[field.Name].Index
			return nil
		}
		if field.ElementWhich != schema.Type_Which_data {
			return errnie.Error(errnie.Err(errnie.Validation, "program: raw entry must carry Data", nil))
		}
		list, listErr := capnp.NewDataList(args.Segment(), 1)

		if listErr != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "program: input list", listErr))
		}

		if err = list.Set(0, payload); err == nil {
			err = args.SetPtr(uint16(field.Offset), list.ToPtr())
		}
	default:
		return errnie.Error(errnie.Err(errnie.Validation, "program: raw entry must carry Data", nil))
	}

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "program: seed input", err))
	}
	frames[index].ready |= 1 << p.Nodes[index].Inputs[field.Name].Index
	return nil
}

/* entry resolves an existing schema input rather than inventing a boundary type. */
func (p *Program) entry(address string) (NodeID, FieldInfo, error) {
	nodeName, port, found := strings.Cut(address, ".")
	index, exists := p.NodeMap[nodeName]

	if !found || !exists {
		return 0, FieldInfo{}, errnie.Error(errnie.Err(errnie.Validation, "program: unknown entry "+address, nil))
	}
	reflected, err := ReflectInterface(p.Nodes[index].Identity.InterfaceID)

	if err != nil {
		return 0, FieldInfo{}, err
	}
	field, found := resolveInputField(reflected, port)

	if !found {
		return 0, FieldInfo{}, errnie.Error(errnie.Err(errnie.Validation, "program: unknown input "+address, nil))
	}
	return index, field, nil
}

/* seedBindings joins outputs from prior LMAX groups at this exact observation. */
func (p *Program) seedBindings(frames []nodeFrame, input runtime.StageNode_step_Params) (bool, error) {
	bindings, err := input.Bindings()

	if err != nil {
		return false, errnie.Error(errnie.Err(errnie.Validation, "program: bindings", err))
	}
	upstream, err := input.Upstream()

	if err != nil {
		return false, errnie.Error(errnie.Err(errnie.Validation, "program: upstream", err))
	}

	if p.stageWidths == nil {
		p.stageWidths = make(map[NodeID]map[string]int)
	}
	for index := range bindings.Len() {
		if bindings.At(index).Gate() {
			continue
		}
		target, err := bindings.At(index).Target()
		if err != nil {
			return false, errnie.Error(err)
		}
		targetNode, _, _ := strings.Cut(target, ".")
		if p.Bindings != nil {
			if _, found := p.Bindings.Inputs[targetNode]; found {
				continue
			}
		}
		nodeIndex, field, err := p.entry(target)
		if err != nil {
			return false, err
		}
		node := &p.Nodes[nodeIndex]
		if field.ValueList {
			_, port, _ := strings.Cut(target, ".")
			slot, _ := outputSlot(port)
			if p.stageWidths[nodeIndex] == nil {
				p.stageWidths[nodeIndex] = make(map[string]int)
			}
			p.stageWidths[nodeIndex][field.Name] = max(p.stageWidths[nodeIndex][field.Name], slot+1)
		}
		if !field.ValueList {
			node.RequiredMask |= 1 << node.Inputs[field.Name].Index
		}
		if !node.Source && !node.Standing && !node.Queued {
			node.Origin = false
		}
	}
	ready := bindings.Len() == 0
	for index := range bindings.Len() {
		present, err := p.seedBinding(frames, bindings.At(index), upstream)
		if err != nil {
			return false, err
		}
		if bindings.At(index).Gate() && !present {
			return false, nil
		}
		ready = ready || present
	}
	return ready, nil
}

/* seedBinding copies a schema-checked active result into its authored target. */
func (p *Program) seedBinding(frames []nodeFrame, binding runtime.Binding, upstream runtime.Result_List) (bool, error) {
	producer, err := binding.Producer()

	if err != nil {
		return false, err
	}
	name, err := binding.Node()

	if err != nil {
		return false, err
	}

	for index := range upstream.Len() {
		result := upstream.At(index)
		source, err := result.Producer()

		if err != nil {
			return false, err
		}
		node, err := result.Node()

		if err != nil {
			return false, err
		}

		if producer != source || name != node {
			continue
		}
		return p.copyBinding(frames, binding, result)
	}
	return false, nil
}

/* copyBinding preserves the original Cap'n Proto value and union presence. */
func (p *Program) copyBinding(frames []nodeFrame, binding runtime.Binding, result runtime.Result) (bool, error) {
	if binding.Stamp() != runtime.Stamp_value {
		return p.copyStamp(frames, binding, result)
	}
	source, err := ReflectInterface(result.InterfaceId())

	if err != nil {
		return false, err
	}
	name, err := binding.Field()

	if err != nil {
		return false, err
	}
	field, found := resolveOutputField(source, name)

	if !found {
		return false, errnie.Error(errnie.Err(errnie.Validation, "program: unknown result field "+name, nil))
	}
	pointer, err := result.Value()

	if err != nil {
		return false, err
	}
	value := pointer.Struct()

	if field.InUnion && value.Uint16(capnp.DataOffset(field.DiscriminantOffset*2)) != field.DiscriminantValue {
		return false, nil
	}
	if binding.Gate() {
		if field.Which == schema.Type_Which_data {
			pointer, err := value.Ptr(uint16(field.Offset))
			if err != nil {
				return false, errnie.Error(err)
			}
			return len(pointer.Data()) > 0, nil
		}
		return true, nil
	}
	target, err := binding.Target()

	if err != nil {
		return false, err
	}
	targetNode, prop, _ := strings.Cut(target, ".")
	if p.Bindings != nil {
		if origin, found := p.Bindings.Inputs[targetNode]; found {
			projection := &resultProjection{nodes: make(map[uint64]schema.Node)}
			projected, err := projection.Field(value, field.SchemaField)
			if err != nil {
				return false, err
			}
			encoded, present, err := boundJSON(projected)
			if err != nil {
				return false, err
			}
			if present {
				p.stageArrivals = append(p.stageArrivals, bindingArrival{BindingEntry{TargetGraph: origin.Definition, TargetComponent: origin.Node, TargetProp: prop}, encoded})
			}
			return present, nil
		}
	}
	index, destination, err := p.entry(target)

	if err != nil {
		return false, err
	}
	var copier Copier
	if field.ValueList && !destination.ValueList {
		slot, numbered := outputSlot(name)
		if !numbered {
			return false, errnie.Error(errnie.Err(errnie.Validation, "program: list output requires a numbered slot: "+name, nil))
		}
		presence, carried := resolveOutputField(source, presencePort)
		if !CompileFanOutDelivery(field, slot, presence, carried)(value) {
			return false, nil
		}
		copier, err = CompileFanOutCopier(field, destination, slot, presence, carried)
	}
	if !field.ValueList && destination.ValueList {
		reflected, reflectErr := ReflectInterface(p.Nodes[index].Identity.InterfaceID)
		if reflectErr != nil {
			return false, reflectErr
		}
		var presence *FieldInfo
		if reported, found := reflected.Inputs[presencePort]; found && destination.ElementWhich == schema.Type_Which_float64 {
			presence = &reported
		}
		_, port, _ := strings.Cut(target, ".")
		slot, _ := outputSlot(port)
		copier, err = CompileFanInCopier(field, destination, presence, slot, p.stageWidths[index][destination.Name])
	}
	if field.ValueList == destination.ValueList {
		copier, err = CompileCopier(field, destination)
	}
	if err != nil {
		return false, err
	}
	args, err := p.arguments(frames, index)

	if err != nil {
		return false, err
	}

	if err := copier(value, args); err != nil {
		return false, err
	}
	frames[index].ready |= 1 << p.Nodes[index].Inputs[destination.Name].Index
	return true, nil
}

/* exportStep hands selected real done results to the next native group. */
func (p *Program) exportStep(call runtime.StageNode_step) error {
	selected, err := call.Args().Outputs()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "program: output selection", err))
	}
	names := make([]string, 0, selected.Len())

	for index := range selected.Len() {
		name, err := selected.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "program: output name", err))
		}

		if _, found := p.NodeMap[name]; !found {
			return errnie.Error(errnie.Err(errnie.Validation, "program: unknown output node "+name, nil))
		}

		if _, found := p.results[name]; found {
			names = append(names, name)
		}
	}
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "program: allocate outputs", err))
	}
	outputs, err := results.NewOutputs(int32(len(names)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "program: allocate output list", err))
	}

	for index, name := range names {
		result := p.results[name]
		output := outputs.At(index)
		output.SetInterfaceId(p.Nodes[p.NodeMap[name]].Identity.InterfaceID)

		if err := output.SetNode(name); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "program: output identity", err))
		}

		if err := output.SetValue(result.ToPtr()); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "program: output value", err))
		}
	}
	frame, err := p.bindingsLocked()

	if err != nil {
		return err
	}
	return results.SetBindings(frame)
}

/* Shutdown releases a definition's graph when its capability is released. */
func (p *Program) Shutdown() { p.Release() }

/* Fence persists the actual graph owners before its consumer releases them. */
func (p *Program) Fence(ctx context.Context, call runtime.StageNode_fence) error { return p.Flush(ctx) }

/* copyStamp binds the producing Result's protocol stamp to an authored Int64 input. */
func (p *Program) copyStamp(frames []nodeFrame, binding runtime.Binding, result runtime.Result) (bool, error) {
	target, err := binding.Target()
	if err != nil {
		return false, errnie.Error(err)
	}
	index, field, err := p.entry(target)
	if err != nil {
		return false, err
	}
	if field.Which != schema.Type_Which_int64 {
		return false, errnie.Error(errnie.Err(errnie.Validation, "program: stamp target must be Int64", nil))
	}
	value := result.Epoch()
	if binding.Stamp() == runtime.Stamp_sequence {
		value = result.Sequence()
	}
	args, err := p.arguments(frames, index)
	if err != nil {
		return false, err
	}
	args.SetUint64(capnp.DataOffset(field.Offset*8), uint64(value))
	frames[index].ready |= 1 << p.Nodes[index].Inputs[field.Name].Index
	return true, nil
}
