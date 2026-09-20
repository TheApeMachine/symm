package tables

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"fmt"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

// IcebergTableServer is a general Iceberg persistence Server.
type IcebergTableServer struct {
	mu          sync.Mutex
	initialized bool
	tableConfig TableConfig
	catalog     *Catalog
}

func NewIcebergTableServer() *IcebergTableServer {
	return &IcebergTableServer{}
}

func (s *IcebergTableServer) Execute(ctx context.Context, call IcebergTable_execute) error {
	args, err := call.Args().Table()
	if err != nil {
		return err
	}

	payloadPtr, err := args.Payload()
	if err != nil || !payloadPtr.IsValid() {
		return nil
	}

	s.mu.Lock()
	if !s.initialized {
		cfgJSON, err := args.Config()
		if err == nil && cfgJSON != "" {
			if err := sonic.Unmarshal([]byte(cfgJSON), &s.tableConfig); err != nil {
				errnie.Error(errnie.Err(errnie.Validation, "iceberg table: invalid config JSON", err))
			}
		}
		s.catalog = Open(context.Background())
		s.initialized = true
	}
	cat := s.catalog
	s.mu.Unlock()

	if cat == nil {
		res, err := call.AllocResults()
		if err == nil {
			res.SetResult(payloadPtr)
		}
		return nil
	}

	schema, err := SchemaFromJSON(fmt.Sprintf(`{"fields":%s}`, toJSON(s.tableConfig.Fields)))
	if err != nil || schema == nil {
		res, err := call.AllocResults()
		if err == nil {
			res.SetResult(payloadPtr)
		}
		return nil
	}

	// Payload processing logic to go here
	// ...

	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	res.SetResult(payloadPtr)
	return nil
}

// IcebergScanServer is a general Iceberg query Server.
type IcebergScanServer struct{}

func NewIcebergScanServer() *IcebergScanServer {
	return &IcebergScanServer{}
}

func (s *IcebergScanServer) Execute(ctx context.Context, call IcebergScan_execute) error {
	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err == nil {
		emptyBytes, _ := sonic.Marshal([]map[string]any{})
		dataPtr, err := capnp.NewData(seg, emptyBytes)
		if err == nil {
			_ = msg.SetRoot(dataPtr.ToPtr())
			res.SetResults(dataPtr.ToPtr())
		}
	}

	return nil
}

func toJSON(v any) string {
	data, _ := sonic.Marshal(v)
	return string(data)
}



type IcebergTableNode types.StreamNode[any, any]

func NewIcebergTable() IcebergTableNode {
	server := &IcebergTableServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}



type IcebergScanNode types.StreamNode[any, any]

func NewIcebergScan() IcebergScanNode {
	server := &IcebergScanServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
