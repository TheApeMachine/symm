package compiler

import (
	"bytes"
	"fmt"
	"math"
	"strconv"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/schemas"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
resultProjection owns schema lookup while projecting one execution's results.
It preserves active union fields and schema defaults. Int64 and UInt64 become
decimal strings so browser JSON parsing cannot round them. Data remains bytes
(encoded as base64 by JSON). Record resolves its registered type ID; other
untyped pointers and capabilities are not UI data.
*/
type resultProjection struct {
	nodes map[uint64]schema.Node
}

/*
Field reads a declared slot using the protocol's XOR defaults.
*/
func (projection *resultProjection) Field(value capnp.Struct, field schema.Field) (any, error) {
	slot := field.Slot()
	valueType, err := slot.Type()

	if err != nil {
		return nil, projectionError("read slot type", err)
	}
	defaults, err := slot.DefaultValue()

	if err != nil {
		return nil, projectionError("read slot default", err)
	}
	offset := slot.Offset()

	switch valueType.Which() {
	case schema.Type_Which_void:
		return nil, nil
	case schema.Type_Which_bool:
		return value.Bit(capnp.BitOffset(offset)) != defaults.Bool(), nil
	case schema.Type_Which_int8:
		return int8(value.Uint8(capnp.DataOffset(offset))) ^ defaults.Int8(), nil
	case schema.Type_Which_uint8:
		return value.Uint8(capnp.DataOffset(offset)) ^ defaults.Uint8(), nil
	case schema.Type_Which_int16:
		return int16(value.Uint16(capnp.DataOffset(offset*2))) ^ defaults.Int16(), nil
	case schema.Type_Which_uint16:
		return value.Uint16(capnp.DataOffset(offset*2)) ^ defaults.Uint16(), nil
	case schema.Type_Which_enum:
		return value.Uint16(capnp.DataOffset(offset*2)) ^ defaults.Enum(), nil
	case schema.Type_Which_int32:
		return int32(value.Uint32(capnp.DataOffset(offset*4))) ^ defaults.Int32(), nil
	case schema.Type_Which_uint32:
		return value.Uint32(capnp.DataOffset(offset*4)) ^ defaults.Uint32(), nil
	case schema.Type_Which_float32:
		return math.Float32frombits(value.Uint32(capnp.DataOffset(offset*4)) ^ math.Float32bits(defaults.Float32())), nil
	case schema.Type_Which_int64:
		return strconv.FormatInt(int64(value.Uint64(capnp.DataOffset(offset*8)))^defaults.Int64(), 10), nil
	case schema.Type_Which_uint64:
		return strconv.FormatUint(value.Uint64(capnp.DataOffset(offset*8))^defaults.Uint64(), 10), nil
	case schema.Type_Which_float64:
		return math.Float64frombits(value.Uint64(capnp.DataOffset(offset*8)) ^ math.Float64bits(defaults.Float64())), nil
	}
	pointer, err := value.Ptr(uint16(offset))

	if err != nil {
		return nil, projectionError("read slot pointer", err)
	}

	if !pointer.IsValid() {
		switch valueType.Which() {
		case schema.Type_Which_text:
			return defaults.Text()
		case schema.Type_Which_data:
			return defaults.Data()
		case schema.Type_Which_list:
			pointer, err = defaults.List()
		case schema.Type_Which_structType:
			pointer, err = defaults.StructValue()
		}

		if err != nil {
			return nil, projectionError("read pointer default", err)
		}
	}
	return projection.Pointer(pointer, valueType)
}

/*
Pointer projects typed pointer data without inventing values for absent pointers.
*/
func (projection *resultProjection) Pointer(pointer capnp.Ptr, valueType schema.Type) (any, error) {
	switch valueType.Which() {
	case schema.Type_Which_text:
		if pointer.IsValid() && pointer.TextBytes() == nil {
			return nil, projectionError("expected text pointer", nil)
		}
		return pointer.Text(), nil
	case schema.Type_Which_data:
		if pointer.IsValid() && pointer.Data() == nil {
			return nil, projectionError("expected data pointer", nil)
		}
		return bytes.Clone(pointer.Data()), nil
	case schema.Type_Which_structType:
		if !pointer.IsValid() {
			return nil, nil
		}

		if !pointer.Struct().IsValid() {
			return nil, projectionError("expected struct pointer", nil)
		}
		return projection.Struct(pointer.Struct(), valueType.StructType().TypeId())
	case schema.Type_Which_list:
		if !pointer.IsValid() {
			return nil, nil
		}

		if !pointer.List().IsValid() {
			return nil, projectionError("expected list pointer", nil)
		}
		element, err := valueType.List().ElementType()

		if err != nil {
			return nil, projectionError("read list element type", err)
		}
		return projection.List(pointer.List(), element)
	default:
		return nil, projectionError(fmt.Sprintf("type %s cannot be projected as UI data", valueType.Which()), nil)
	}
}

/*
Struct projects declared fields, including groups, omitting inactive union arms.
*/
func (projection *resultProjection) Struct(value capnp.Struct, typeID uint64) (map[string]any, error) {
	if typeID == types.Record_TypeID {
		record := types.Record(value)
		pointer, err := record.Value()
		if err != nil {
			return nil, projectionError("read native record", err)
		}
		if !pointer.Struct().IsValid() {
			return nil, projectionError("native record requires a typed struct value", nil)
		}
		projected, err := projection.Struct(pointer.Struct(), record.TypeId())
		if err != nil {
			return nil, err
		}
		return map[string]any{"typeId": strconv.FormatUint(record.TypeId(), 10), "value": projected}, nil
	}
	node, err := projection.Node(typeID)

	if err != nil {
		return nil, err
	}
	if node.Which() != schema.Node_Which_structNode {
		return nil, projectionError(fmt.Sprintf("schema %x is not a struct", typeID), nil)
	}
	discriminants := node.StructNode().DiscriminantCount()

	if discriminants > 0 && value.Uint16(capnp.DataOffset(node.StructNode().DiscriminantOffset()*2)) >= discriminants {
		return nil, projectionError("unknown union discriminator", nil)
	}

	fields, err := node.StructNode().Fields()

	if err != nil {
		return nil, projectionError("read struct fields", err)
	}
	result := make(map[string]any)
	for index := range fields.Len() {
		field := fields.At(index)

		if field.DiscriminantValue() != schema.Field_noDiscriminant && field.DiscriminantValue() != value.Uint16(capnp.DataOffset(node.StructNode().DiscriminantOffset()*2)) {
			continue
		}
		name, err := field.Name()

		if err != nil {
			return nil, projectionError("read field name", err)
		}
		var projected any

		if field.Which() == schema.Field_Which_group {
			projected, err = projection.Struct(value, field.Group().TypeId())
		}

		if field.Which() == schema.Field_Which_slot {
			projected, err = projection.Field(value, field)
		}

		if err != nil {
			return nil, projectionError("field "+name, err)
		}
		result[name] = projected
	}
	return result, nil
}

/*
List uses Cap'n Proto's typed list accessors, including recursive and struct lists.
*/
func (projection *resultProjection) List(values capnp.List, element schema.Type) ([]any, error) {
	result := make([]any, values.Len())
	for index := range values.Len() {
		switch element.Which() {
		case schema.Type_Which_void:
			result[index] = nil
		case schema.Type_Which_bool:
			result[index] = capnp.BitList(values).At(index)
		case schema.Type_Which_int8:
			result[index] = capnp.Int8List(values).At(index)
		case schema.Type_Which_uint8:
			result[index] = capnp.UInt8List(values).At(index)
		case schema.Type_Which_int16:
			result[index] = capnp.Int16List(values).At(index)
		case schema.Type_Which_uint16, schema.Type_Which_enum:
			result[index] = capnp.UInt16List(values).At(index)
		case schema.Type_Which_int32:
			result[index] = capnp.Int32List(values).At(index)
		case schema.Type_Which_uint32:
			result[index] = capnp.UInt32List(values).At(index)
		case schema.Type_Which_int64:
			result[index] = strconv.FormatInt(capnp.Int64List(values).At(index), 10)
		case schema.Type_Which_uint64:
			result[index] = strconv.FormatUint(capnp.UInt64List(values).At(index), 10)
		case schema.Type_Which_float32:
			result[index] = capnp.Float32List(values).At(index)
		case schema.Type_Which_float64:
			result[index] = capnp.Float64List(values).At(index)
		case schema.Type_Which_structType:
			projected, err := projection.Struct(values.Struct(index), element.StructType().TypeId())

			if err != nil {
				return nil, projectionError(fmt.Sprintf("list element %d", index), err)
			}
			result[index] = projected
		default:
			pointer, err := capnp.PointerList(values).At(index)

			if err != nil {
				return nil, projectionError(fmt.Sprintf("read list element %d", index), err)
			}
			projected, err := projection.Pointer(pointer, element)

			if err != nil {
				return nil, projectionError(fmt.Sprintf("list element %d", index), err)
			}
			result[index] = projected
		}
	}
	return result, nil
}

/*
Node resolves schema IDs through the existing registry, including imported types.
*/
func (projection *resultProjection) Node(typeID uint64) (schema.Node, error) {
	if node, found := projection.nodes[typeID]; found {
		return node, nil
	}
	raw, err := schemas.DefaultRegistry.Find(typeID)

	if err != nil {
		return schema.Node{}, projectionError(fmt.Sprintf("find schema %x", typeID), err)
	}
	message, err := capnp.Unmarshal(raw)

	if err != nil {
		return schema.Node{}, projectionError("decode schema", err)
	}
	// Registered schema metadata is trusted and repeatedly read for each element.
	// The separate result message retains its normal traversal limit.
	message.ResetReadLimit(math.MaxUint64)
	request, err := schema.ReadRootCodeGeneratorRequest(message)

	if err != nil {
		return schema.Node{}, projectionError("read schema", err)
	}
	nodes, err := request.Nodes()

	if err != nil {
		return schema.Node{}, projectionError("read schema nodes", err)
	}
	for index := range nodes.Len() {
		node := nodes.At(index)
		projection.nodes[node.Id()] = node
	}
	node, found := projection.nodes[typeID]

	if !found {
		return schema.Node{}, projectionError(fmt.Sprintf("schema %x missing from registry payload", typeID), nil)
	}
	return node, nil
}

func projectionError(message string, err error) error {
	return errnie.Error(errnie.Err(errnie.Validation, "result projection: "+message, err))
}
