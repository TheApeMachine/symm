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
	Name               string
	Offset             uint32
	Which              schema.Type_Which
	InUnion            bool
	DiscriminantValue  uint16
	DiscriminantOffset uint32
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
				fields, err := st.Fields()
				if err == nil {
					for k := 0; k < fields.Len(); k++ {
						f := fields.At(k)
						fName, _ := f.Name()
						slot := f.Slot()
						t, _ := slot.Type()
						discVal := f.DiscriminantValue()
						result.Outputs[fName] = FieldInfo{
							Name:               fName,
							Offset:             slot.Offset(),
							Which:              t.Which(),
							InUnion:            discVal != schema.Field_noDiscriminant,
							DiscriminantValue:  discVal,
							DiscriminantOffset: st.DiscriminantOffset(),
						}
					}
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

	case schema.Type_Which_int16, schema.Type_Which_uint16:
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
