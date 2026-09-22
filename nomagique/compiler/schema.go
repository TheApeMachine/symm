package compiler

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/schemas"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/theapemachine/errnie"
)

/*
FieldInfo contains reflected Cap'n Proto field metadata.
*/
type FieldInfo struct {
	SchemaField        schema.Field
	Name               string
	Offset             uint32
	Which              schema.Type_Which
	InUnion            bool
	DiscriminantValue  uint16
	DiscriminantOffset uint32
	// InterfaceID names the interface a capability field requires, and is
	// zero for every field carrying a value.
	InterfaceID uint64
	// CapabilityList marks a field that carries a list of capabilities rather
	// than one, so every node wired into it is kept instead of replacing the
	// node wired before it.
	CapabilityList bool
	// ValueList marks a field that carries a list of values, which is how
	// several producers land on one port without overwriting each other.
	ValueList bool
	// ElementWhich is the type carried by a list field's elements.
	ElementWhich schema.Type_Which
}

/*
InterfaceSchema contains the reflected Cap'n Proto interface protocol.
*/
type InterfaceSchema struct {
	InterfaceID uint64
	Name        string
	WriteMethod uint16
	WriteParams capnp.ObjectSize
	WriteResult capnp.ObjectSize
	Inputs      map[string]FieldInfo
	DoneMethod  uint16
	DoneParams  capnp.ObjectSize
	DoneResult  capnp.ObjectSize
	Outputs     map[string]FieldInfo
	HasWrite    bool
	HasDone     bool
}

/*
Copier copies a field from src struct to dst struct without payload boxing or any.
*/
type Copier func(src capnp.Struct, dst capnp.Struct) error

var (
	schemaCacheMu sync.RWMutex
	schemaCache   = make(map[uint64]*InterfaceSchema)
)

/*
ReflectInterface loads and caches the Cap'n Proto interface schema from schemas.DefaultRegistry.
*/
func ReflectInterface(interfaceID uint64) (*InterfaceSchema, error) {
	schemaCacheMu.RLock()
	cached, found := schemaCache[interfaceID]
	schemaCacheMu.RUnlock()
	if found {
		return cached, nil
	}

	schemaCacheMu.Lock()
	defer schemaCacheMu.Unlock()

	// Check again under lock
	if cached, found := schemaCache[interfaceID]; found {
		return cached, nil
	}

	raw, err := schemas.DefaultRegistry.Find(interfaceID)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("compiler: Cap'n Proto schema not found for interface ID %x", interfaceID),
			err,
		))
	}

	msg, err := capnp.Unmarshal(raw)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("compiler: failed to unmarshal schema for interface ID %x", interfaceID),
			err,
		))
	}

	// This immutable generated schema is cached across executions. Its read
	// budget must not expire as callers inspect metadata; payloads retain theirs.
	msg.ResetReadLimit(math.MaxUint64)
	req, err := schema.ReadRootCodeGeneratorRequest(msg)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("compiler: failed to read CodeGeneratorRequest for interface ID %x", interfaceID),
			err,
		))
	}

	nodes, err := req.Nodes()
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("compiler: failed to read nodes for interface ID %x", interfaceID),
			err,
		))
	}

	nodeMap := make(map[uint64]schema.Node, nodes.Len())
	for i := 0; i < nodes.Len(); i++ {
		n := nodes.At(i)
		nodeMap[n.Id()] = n
	}

	ifaceNode, exists := nodeMap[interfaceID]
	if !exists {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("compiler: interface node %x not found in schema", interfaceID),
			nil,
		))
	}

	ifaceName, _ := ifaceNode.DisplayName()
	iface := ifaceNode.Interface()
	methods, err := iface.Methods()
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("compiler: failed to read methods for interface %s", ifaceName),
			err,
		))
	}

	result := &InterfaceSchema{
		InterfaceID: interfaceID,
		Name:        ifaceName,
		Inputs:      make(map[string]FieldInfo),
		Outputs:     make(map[string]FieldInfo),
	}

	for i := 0; i < methods.Len(); i++ {
		m := methods.At(i)
		mName, _ := m.Name()

		if mName == "write" || (i == 0 && mName == "") {
			result.HasWrite = true
			result.WriteMethod = uint16(i)
			paramNode := nodeMap[m.ParamStructType()]
			if paramNode.IsValid() {
				st := paramNode.StructNode()
				result.WriteParams = capnp.ObjectSize{
					DataSize:     capnp.Size(st.DataWordCount() * 8),
					PointerCount: uint16(st.PointerCount()),
				}
				fields, err := st.Fields()
				if err == nil {
					for k := 0; k < fields.Len(); k++ {
						f := fields.At(k)
						fName, _ := f.Name()
						slot := f.Slot()
						t, _ := slot.Type()
						discVal := f.DiscriminantValue()
						result.Inputs[fName] = FieldInfo{
							Name:               fName,
							Offset:             slot.Offset(),
							Which:              t.Which(),
							InUnion:            discVal != schema.Field_noDiscriminant,
							DiscriminantValue:  discVal,
							DiscriminantOffset: st.DiscriminantOffset(),
							InterfaceID:        requiredInterface(t),
							CapabilityList:     isCapabilityList(t),
							ValueList:          isValueList(t),
							ElementWhich:       elementWhich(t),
						}
					}
				}
			}
			resNode := nodeMap[m.ResultStructType()]
			if resNode.IsValid() {
				st := resNode.StructNode()
				result.WriteResult = capnp.ObjectSize{
					DataSize:     capnp.Size(st.DataWordCount() * 8),
					PointerCount: uint16(st.PointerCount()),
				}
			}
		}

		if mName == "done" || (i == 1 && mName == "") {
			result.DoneMethod = uint16(i)
			result.HasDone = true
			paramNode := nodeMap[m.ParamStructType()]
			if paramNode.IsValid() {
				st := paramNode.StructNode()
				result.DoneParams = capnp.ObjectSize{
					DataSize:     capnp.Size(st.DataWordCount() * 8),
					PointerCount: uint16(st.PointerCount()),
				}
			}
			resNode := nodeMap[m.ResultStructType()]
			if resNode.IsValid() {
				st := resNode.StructNode()
				result.DoneResult = capnp.ObjectSize{
					DataSize:     capnp.Size(st.DataWordCount() * 8),
					PointerCount: uint16(st.PointerCount()),
				}
				result.Outputs, err = reflectOutputFields(resNode, nodeMap, "", nil)

				if err != nil {
					return nil, err
				}

			}
		}
	}

	schemaCache[interfaceID] = result
	return result, nil
}

/*
CompileCopier creates a typed copy operation from source result field to destination argument field.
*/
func CompileCopier(fromField FieldInfo, toField FieldInfo) (Copier, error) {
	if fromField.Which != toField.Which {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: type mismatch on field %s (%v) -> %s (%v)",
				fromField.Name, fromField.Which, toField.Name, toField.Which),
			nil,
		))
	}

	which := fromField.Which
	fromOffset := fromField.Offset
	toOffset := toField.Offset

	switch which {
	case schema.Type_Which_void:
		return nil, nil

	case schema.Type_Which_bool:
		fromBit := capnp.BitOffset(fromOffset)
		toBit := capnp.BitOffset(toOffset)
		return func(src, dst capnp.Struct) error {
			dst.SetBit(toBit, src.Bit(fromBit))
			return nil
		}, nil

	case schema.Type_Which_int8, schema.Type_Which_uint8:
		fromOff := capnp.DataOffset(fromOffset)
		toOff := capnp.DataOffset(toOffset)
		return func(src, dst capnp.Struct) error {
			dst.SetUint8(toOff, src.Uint8(fromOff))
			return nil
		}, nil

	case schema.Type_Which_int16, schema.Type_Which_uint16, schema.Type_Which_enum:
		fromOff := capnp.DataOffset(fromOffset * 2)
		toOff := capnp.DataOffset(toOffset * 2)
		return func(src, dst capnp.Struct) error {
			dst.SetUint16(toOff, src.Uint16(fromOff))
			return nil
		}, nil

	case schema.Type_Which_int32, schema.Type_Which_uint32:
		fromOff := capnp.DataOffset(fromOffset * 4)
		toOff := capnp.DataOffset(toOffset * 4)
		return func(src, dst capnp.Struct) error {
			dst.SetUint32(toOff, src.Uint32(fromOff))
			return nil
		}, nil

	case schema.Type_Which_float32:
		fromOff := capnp.DataOffset(fromOffset * 4)
		toOff := capnp.DataOffset(toOffset * 4)
		return func(src, dst capnp.Struct) error {
			dst.SetUint32(toOff, src.Uint32(fromOff))
			return nil
		}, nil

	case schema.Type_Which_int64, schema.Type_Which_uint64:
		fromOff := capnp.DataOffset(fromOffset * 8)
		toOff := capnp.DataOffset(toOffset * 8)
		return func(src, dst capnp.Struct) error {
			dst.SetUint64(toOff, src.Uint64(fromOff))
			return nil
		}, nil

	case schema.Type_Which_float64:
		fromOff := capnp.DataOffset(fromOffset * 8)
		toOff := capnp.DataOffset(toOffset * 8)
		return func(src, dst capnp.Struct) error {
			dst.SetUint64(toOff, src.Uint64(fromOff))
			return nil
		}, nil

	case schema.Type_Which_text, schema.Type_Which_data,
		schema.Type_Which_structType, schema.Type_Which_list,
		schema.Type_Which_anyPointer, schema.Type_Which_interface:
		fromPtr := uint16(fromOffset)
		toPtr := uint16(toOffset)
		return func(src, dst capnp.Struct) error {
			p, err := src.Ptr(fromPtr)
			if err != nil {
				return err
			}
			return dst.SetPtr(toPtr, p)
		}, nil

	default:
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: unsupported Cap'n Proto type kind %v for copier", which),
			nil,
		))
	}
}

/*
SetStaticField writes a static string/number into target struct based on field metadata.
*/
func SetStaticField(target capnp.Struct, field FieldInfo, rawVal string) error {
	switch field.Which {
	case schema.Type_Which_bool:
		b, err := strconv.ParseBool(rawVal)
		if err != nil {
			return err
		}
		target.SetBit(capnp.BitOffset(field.Offset), b)
		return nil

	case schema.Type_Which_float64:
		f, err := strconv.ParseFloat(rawVal, 64)
		if err != nil {
			return err
		}
		target.SetUint64(capnp.DataOffset(field.Offset*8), math.Float64bits(f))
		return nil

	case schema.Type_Which_float32:
		f, err := strconv.ParseFloat(rawVal, 32)
		if err != nil {
			return err
		}
		target.SetUint32(capnp.DataOffset(field.Offset*4), math.Float32bits(float32(f)))
		return nil

	case schema.Type_Which_int64:
		i, err := strconv.ParseInt(rawVal, 10, 64)
		if err != nil {
			return err
		}
		target.SetUint64(capnp.DataOffset(field.Offset*8), uint64(i))
		return nil

	case schema.Type_Which_uint64:
		u, err := strconv.ParseUint(rawVal, 10, 64)
		if err != nil {
			return err
		}
		target.SetUint64(capnp.DataOffset(field.Offset*8), u)
		return nil

	case schema.Type_Which_int32:
		i, err := strconv.ParseInt(rawVal, 10, 32)
		if err != nil {
			return err
		}
		target.SetUint32(capnp.DataOffset(field.Offset*4), uint32(i))
		return nil

	case schema.Type_Which_uint32:
		u, err := strconv.ParseUint(rawVal, 10, 32)
		if err != nil {
			return err
		}
		target.SetUint32(capnp.DataOffset(field.Offset*4), uint32(u))
		return nil

	case schema.Type_Which_int16, schema.Type_Which_uint16, schema.Type_Which_enum:
		u, err := strconv.ParseUint(rawVal, 10, 16)
		if err != nil {
			return err
		}
		target.SetUint16(capnp.DataOffset(field.Offset*2), uint16(u))
		return nil

	case schema.Type_Which_text:
		return target.SetText(uint16(field.Offset), rawVal)

	case schema.Type_Which_data:
		return target.SetData(uint16(field.Offset), []byte(rawVal))

	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: cannot set static field of type %v", field.Which),
			nil,
		))
	}
}

/*
requiredInterface names the interface a field requires when it carries a
capability, and reports zero for a field carrying a value.
*/
func requiredInterface(fieldType schema.Type) uint64 {
	if fieldType.Which() == schema.Type_Which_interface {
		return fieldType.Interface().TypeId()
	}

	element, listed := capabilityElement(fieldType)

	if !listed {
		return 0
	}

	return element
}

/*
capabilityElement names the interface a list field carries, reporting false for
a list of values and for anything that is not a list.
*/
func capabilityElement(fieldType schema.Type) (uint64, bool) {
	if fieldType.Which() != schema.Type_Which_list {
		return 0, false
	}

	element, err := fieldType.List().ElementType()

	if err != nil || element.Which() != schema.Type_Which_interface {
		return 0, false
	}

	return element.Interface().TypeId(), true
}

/*
Implements reports whether an interface satisfies a required one, which it
does when they are the same interface or when the required interface is among
its superclasses. Cap'n Proto interfaces are nominally typed, so a node is
wired into a capability port by declaring that it extends what the port
requires, never by happening to carry matching methods.
*/
func Implements(interfaceID, required uint64) bool {
	if required == 0 || interfaceID == required {
		return true
	}

	raw, err := schemas.DefaultRegistry.Find(interfaceID)

	if err != nil {
		return false
	}

	message, err := capnp.Unmarshal(raw)

	if err != nil {
		return false
	}

	root, err := schema.ReadRootCodeGeneratorRequest(message)

	if err != nil {
		return false
	}

	nodes, err := root.Nodes()

	if err != nil {
		return false
	}

	for index := 0; index < nodes.Len(); index++ {
		node := nodes.At(index)

		if node.Id() != interfaceID || node.Which() != schema.Node_Which_interface {
			continue
		}

		superclasses, err := node.Interface().Superclasses()

		if err != nil {
			return false
		}

		for position := 0; position < superclasses.Len(); position++ {
			if superclasses.At(position).Id() == required {
				return true
			}
		}
	}

	return false
}

/*
isCapabilityList reports a field that carries a list of capabilities.
*/
func isCapabilityList(fieldType schema.Type) bool {
	_, listed := capabilityElement(fieldType)
	return listed
}

/*
elementWhich names the type a list field's elements carry, and reports the
field's own type when it is not a list.
*/
func elementWhich(fieldType schema.Type) schema.Type_Which {
	if fieldType.Which() != schema.Type_Which_list {
		return fieldType.Which()
	}

	element, err := fieldType.List().ElementType()

	if err != nil {
		return fieldType.Which()
	}

	return element.Which()
}

/*
isValueList reports a field that carries a list of values, which several
producers can land on at once.
*/
func isValueList(fieldType schema.Type) bool {
	if fieldType.Which() != schema.Type_Which_list {
		return false
	}

	return !isCapabilityList(fieldType)
}

/*
CompileFanInCopier builds the transfer for one producer landing on a port that
gathers several. The producers share the list, each writing the slot it was
given, so arriving in any order leaves every value in place.
*/
func CompileFanInCopier(
	fromField FieldInfo,
	toField FieldInfo,
	index int,
	length int,
) (Copier, error) {
	if fromField.Which != toField.ElementWhich {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: type mismatch on field %s (%v) -> %s element (%v)",
				fromField.Name, fromField.Which, toField.Name, toField.ElementWhich),
			nil,
		))
	}

	fromOffset := uint16(fromField.Offset)
	toOffset := uint16(toField.Offset)

	return func(src, dst capnp.Struct) error {
		gathered, err := fanInList(dst, toOffset, length)

		if err != nil {
			return err
		}

		value, err := src.Ptr(fromOffset)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"compiler: failed to read value landing on a gathering port",
				err,
			))
		}

		if err := gathered.Set(index, value); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"compiler: failed to place value on a gathering port",
				err,
			))
		}

		return nil
	}, nil
}

/*
fanInList resolves the shared list a gathering port holds, allocating it the
first time a producer lands on it.
*/
func fanInList(dst capnp.Struct, offset uint16, length int) (capnp.PointerList, error) {
	existing, err := dst.Ptr(offset)

	if err != nil {
		return capnp.PointerList{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"compiler: failed to read a gathering port",
			err,
		))
	}

	if existing.IsValid() && existing.List().Len() == length {
		return capnp.PointerList(existing.List()), nil
	}

	gathered, err := capnp.NewPointerList(dst.Segment(), int32(length))

	if err != nil {
		return capnp.PointerList{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"compiler: failed to allocate a gathering port",
			err,
		))
	}

	if err := dst.SetPtr(offset, gathered.ToPtr()); err != nil {
		return capnp.PointerList{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"compiler: failed to attach a gathering port",
			err,
		))
	}

	return gathered, nil
}

/*
CompileFanOutCopier builds the transfer for one consumer reading a single slot
of a port that hands back several values.

A producer that answers many questions at once numbers its answers, so a
consumer takes the slot it asked for rather than the whole reply.
*/
func CompileFanOutCopier(
	fromField FieldInfo,
	toField FieldInfo,
	index int,
	presence FieldInfo,
	carried bool,
) (Copier, error) {
	if fromField.ElementWhich != toField.Which {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: type mismatch on field %s element (%v) -> %s (%v)",
				fromField.Name, fromField.ElementWhich, toField.Name, toField.Which),
			nil,
		))
	}

	fromOffset := uint16(fromField.Offset)
	toOffset := capnp.DataOffset(toField.Offset * 8)
	filled := compileSlotPresence(presence, carried)

	return func(src, dst capnp.Struct) error {
		pointer, err := src.Ptr(fromOffset)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"compiler: failed to read a port handing back several values",
				err,
			))
		}

		handed := capnp.Float64List(pointer.List())

		// A slot the producer did not fill is not a value: leaving it alone
		// keeps the consumer waiting rather than reading a zero as evidence.
		if !handed.IsValid() || index >= handed.Len() {
			return nil
		}

		if !filled(src, index) {
			return nil
		}

		dst.SetUint64(toOffset, math.Float64bits(handed.At(index)))
		return nil
	}, nil
}

/*
compileSlotPresence reads whether a producer filled one of its slots. A
producer that does not report presence filled every slot it handed back.
*/
func compileSlotPresence(
	presence FieldInfo, carried bool,
) func(capnp.Struct, int) bool {
	if !carried {
		return func(capnp.Struct, int) bool { return true }
	}

	offset := uint16(presence.Offset)

	return func(src capnp.Struct, index int) bool {
		pointer, err := src.Ptr(offset)

		if err != nil {
			return false
		}

		reported := capnp.BitList(pointer.List())

		if !reported.IsValid() || index >= reported.Len() {
			return false
		}

		return reported.At(index)
	}
}

/*
CompileFanOutDelivery reports whether a port handing back several values
carried the one a consumer is waiting for.
*/
func CompileFanOutDelivery(
	fromField FieldInfo, index int, presence FieldInfo, carried bool,
) func(capnp.Struct) bool {
	fromOffset := uint16(fromField.Offset)
	filled := compileSlotPresence(presence, carried)

	return func(src capnp.Struct) bool {
		pointer, err := src.Ptr(fromOffset)

		if err != nil {
			return false
		}

		handed := capnp.Float64List(pointer.List())

		if !handed.IsValid() || index >= handed.Len() {
			return false
		}

		return filled(src, index)
	}
}

/*
reflectOutputFields flattens result groups into named ports while retaining their
union discriminator. Every value inside move is absent when none is active.
*/
func reflectOutputFields(node schema.Node, nodes map[uint64]schema.Node, prefix string, inherited *FieldInfo) (map[string]FieldInfo, error) {
	fields, err := node.StructNode().Fields()

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: read result fields", err))
	}

	output := make(map[string]FieldInfo)

	for index := range fields.Len() {
		field := fields.At(index)
		name, err := field.Name()

		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: read result field name", err))
		}

		info := FieldInfo{Name: prefix + name}

		if inherited != nil {
			info.InUnion = inherited.InUnion
			info.DiscriminantValue = inherited.DiscriminantValue
			info.DiscriminantOffset = inherited.DiscriminantOffset
		}

		if field.DiscriminantValue() != schema.Field_noDiscriminant {
			if info.InUnion {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: nested result unions require multiple discriminators", nil))
			}

			info.InUnion = true
			info.DiscriminantValue = field.DiscriminantValue()
			info.DiscriminantOffset = node.StructNode().DiscriminantOffset()
		}

		if field.Which() == schema.Field_Which_group {
			group, err := reflectOutputFields(nodes[field.Group().TypeId()], nodes, info.Name+".", &info)

			if err != nil {
				return nil, err
			}

			for key, value := range group {
				output[key] = value
			}
			continue
		}

		slot := field.Slot()
		valueType, err := slot.Type()

		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: read result field type", err))
		}

		info.SchemaField = field
		info.Offset = slot.Offset()
		info.Which = valueType.Which()
		info.InterfaceID = requiredInterface(valueType)
		info.CapabilityList = isCapabilityList(valueType)
		info.ValueList = isValueList(valueType)
		info.ElementWhich = elementWhich(valueType)
		output[info.Name] = info
	}

	return output, nil
}
