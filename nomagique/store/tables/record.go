package tables

import (
	"bytes"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/schemas"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/* tableRow owns one immutable arrival until its append is acknowledged. */
type tableRow struct {
	raw    []byte
	record types.Record
	size   int
}

/* release relinquishes native storage only after a successful append or shutdown. */
func (row tableRow) release() {
	if row.record.IsValid() {
		row.record.Message().Release()
	}
}

/* nativeColumns compiles a declared Cap'n Proto record into Arrow columns once. */
type nativeColumns struct {
	nodes map[uint64]schema.Node
	plans map[uint64][]nativeColumn
}

/* nativeColumn reads one schema slot without a JSON or untyped-value intermediate. */
type nativeColumn struct {
	read func(array.Builder, capnp.Struct) error
}

/* append writes one native record directly into the existing Arrow batch. */
func (columns *nativeColumns) append(builder *array.RecordBuilder, record types.Record) error {
	plan, found := columns.plans[record.TypeId()]
	if !found {
		var err error
		plan, err = columns.compile(record.TypeId(), builder.Schema().Fields())
		if err != nil {
			return err
		}
		if columns.plans == nil {
			columns.plans = make(map[uint64][]nativeColumn)
		}
		columns.plans[record.TypeId()] = plan
	}
	pointer, err := record.Value()
	if err != nil {
		return errnie.Error(err)
	}
	if !pointer.Struct().IsValid() {
		return recordError("record value must be a struct", nil)
	}
	for index, column := range plan {
		if err := column.read(builder.Field(index), pointer.Struct()); err != nil {
			return err
		}
	}
	return nil
}

/* compile binds column names and types to the authoritative native schema. */
func (columns *nativeColumns) compile(typeID uint64, fields []arrow.Field) ([]nativeColumn, error) {
	node, err := columns.node(typeID)
	if err != nil {
		return nil, err
	}
	declared, err := node.StructNode().Fields()
	if err != nil {
		return nil, errnie.Error(err)
	}
	named := make(map[string]schema.Field, declared.Len())
	for index := range declared.Len() {
		field := declared.At(index)
		name, err := field.Name()
		if err != nil {
			return nil, errnie.Error(err)
		}
		named[name] = field
	}
	plan := make([]nativeColumn, len(fields))
	for index, target := range fields {
		field, found := named[target.Name]
		if !found || field.Which() != schema.Field_Which_slot {
			return nil, recordError("missing native slot for column "+target.Name, nil)
		}
		// Generic records must expose a concrete row, not an inactive union alternative.
		if field.DiscriminantValue() != math.MaxUint16 {
			return nil, recordError("union column requires an explicit concrete record: "+target.Name, nil)
		}
		read, err := columns.slot(field, target.Type)
		if err != nil {
			return nil, err
		}
		plan[index] = nativeColumn{read: read}
	}
	return plan, nil
}

/* slot validates the native/Arrow type pair before constructing its reader. */
func (columns *nativeColumns) slot(field schema.Field, target arrow.DataType) (func(array.Builder, capnp.Struct) error, error) {
	source, err := field.Slot().Type()
	if err != nil {
		return nil, errnie.Error(err)
	}
	defaults, err := field.Slot().DefaultValue()
	if err != nil {
		return nil, errnie.Error(err)
	}
	offset := field.Slot().Offset()
	switch source.Which() {
	case schema.Type_Which_bool:
		if target.ID() == arrow.BOOL {
			return func(builder array.Builder, value capnp.Struct) error {
				builder.(*array.BooleanBuilder).Append(value.Bit(capnp.BitOffset(offset)) != defaults.Bool())
				return nil
			}, nil
		}
	case schema.Type_Which_int64:
		if target.ID() == arrow.INT64 {
			return func(builder array.Builder, value capnp.Struct) error {
				builder.(*array.Int64Builder).Append(int64(value.Uint64(capnp.DataOffset(offset*8))) ^ defaults.Int64())
				return nil
			}, nil
		}
	case schema.Type_Which_int32:
		if target.ID() == arrow.INT32 {
			return func(builder array.Builder, value capnp.Struct) error {
				builder.(*array.Int32Builder).Append(int32(value.Uint32(capnp.DataOffset(offset*4))) ^ defaults.Int32())
				return nil
			}, nil
		}
	case schema.Type_Which_float64:
		if target.ID() == arrow.FLOAT64 {
			return func(builder array.Builder, value capnp.Struct) error {
				builder.(*array.Float64Builder).Append(math.Float64frombits(value.Uint64(capnp.DataOffset(offset*8)) ^ math.Float64bits(defaults.Float64())))
				return nil
			}, nil
		}
	case schema.Type_Which_float32:
		if target.ID() == arrow.FLOAT32 {
			return func(builder array.Builder, value capnp.Struct) error {
				builder.(*array.Float32Builder).Append(math.Float32frombits(value.Uint32(capnp.DataOffset(offset*4)) ^ math.Float32bits(defaults.Float32())))
				return nil
			}, nil
		}
	case schema.Type_Which_text:
		if target.ID() == arrow.STRING {
			return func(builder array.Builder, value capnp.Struct) error {
				pointer, err := value.Ptr(uint16(offset))
				if err != nil {
					return errnie.Error(err)
				}
				text, err := defaults.Text()
				if err != nil {
					return errnie.Error(err)
				}
				builder.(*array.StringBuilder).Append(pointer.TextDefault(text))
				return nil
			}, nil
		}
	case schema.Type_Which_data:
		if target.ID() == arrow.BINARY {
			return func(builder array.Builder, value capnp.Struct) error {
				pointer, err := value.Ptr(uint16(offset))
				if err != nil {
					return errnie.Error(err)
				}
				data, err := defaults.Data()
				if err != nil {
					return errnie.Error(err)
				}
				builder.(*array.BinaryBuilder).Append(pointer.DataDefault(data))
				return nil
			}, nil
		}
	case schema.Type_Which_structType:
		if target.ID() == arrow.STRUCT {
			nested, err := columns.compile(source.StructType().TypeId(), target.(*arrow.StructType).Fields())
			if err != nil {
				return nil, err
			}
			return func(builder array.Builder, value capnp.Struct) error {
				pointer, err := value.Ptr(uint16(offset))
				if err != nil {
					return errnie.Error(err)
				}
				return appendStruct(builder.(*array.StructBuilder), pointer.Struct(), nested)
			}, nil
		}
	case schema.Type_Which_list:
		element, err := source.List().ElementType()
		if err != nil {
			return nil, errnie.Error(err)
		}
		if target.ID() == arrow.LIST && element.Which() == schema.Type_Which_structType && target.(*arrow.ListType).Elem().ID() == arrow.STRUCT {
			nested, err := columns.compile(element.StructType().TypeId(), target.(*arrow.ListType).Elem().(*arrow.StructType).Fields())
			if err != nil {
				return nil, err
			}
			return func(builder array.Builder, value capnp.Struct) error {
				pointer, err := value.Ptr(uint16(offset))
				if err != nil {
					return errnie.Error(err)
				}
				list := pointer.List()
				target := builder.(*array.ListBuilder)
				target.Append(true)
				for index := range list.Len() {
					if err := appendStruct(target.ValueBuilder().(*array.StructBuilder), list.Struct(index), nested); err != nil {
						return err
					}
				}
				return nil
			}, nil
		}
	}
	return nil, recordError("native slot type "+source.Which().String()+" cannot populate "+target.String(), nil)
}

/* appendStruct preserves absence rather than inventing default nested fields. */
func appendStruct(builder *array.StructBuilder, value capnp.Struct, plan []nativeColumn) error {
	if !value.IsValid() {
		builder.AppendNull()
		return nil
	}
	builder.Append(true)
	for index, column := range plan {
		if err := column.read(builder.FieldBuilder(index), value); err != nil {
			return err
		}
	}
	return nil
}

/* node resolves generated schema metadata and retains it for compiled readers. */
func (columns *nativeColumns) node(typeID uint64) (schema.Node, error) {
	if found, exists := columns.nodes[typeID]; exists {
		return found, nil
	}
	raw, err := schemas.DefaultRegistry.Find(typeID)
	if err != nil {
		return schema.Node{}, recordError("unknown record schema", err)
	}
	message, err := capnp.Unmarshal(raw)
	if err != nil {
		return schema.Node{}, recordError("decode record schema", err)
	}
	message.ResetReadLimit(math.MaxUint64)
	request, err := schema.ReadRootCodeGeneratorRequest(message)
	if err != nil {
		return schema.Node{}, recordError("read record schema", err)
	}
	nodes, err := request.Nodes()
	if err != nil {
		return schema.Node{}, recordError("read record schema nodes", err)
	}
	if columns.nodes == nil {
		columns.nodes = make(map[uint64]schema.Node)
	}
	for index := range nodes.Len() {
		node := nodes.At(index)
		columns.nodes[node.Id()] = node
	}
	node, found := columns.nodes[typeID]
	if !found || node.Which() != schema.Node_Which_structNode {
		return schema.Node{}, recordError("record schema is not a struct", nil)
	}
	return node, nil
}

/* recordError identifies a storage boundary mismatch before any append. */
func recordError(message string, err error) error {
	return errnie.Error(errnie.Err(errnie.Validation, "iceberg native record: "+message, err))
}

/* heldRow copies an external JSON row into its immutable storage owner. */
func heldRow(row []byte) tableRow { return tableRow{raw: bytes.Clone(row), size: len(row)} }
