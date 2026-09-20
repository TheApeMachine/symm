package tables

import (
	"bytes"
	"context"
	"sync"

	"github.com/theapemachine/errnie"
)

type IcebergTableServer struct {
	mu          sync.Mutex
	initialized bool
	tableConfig TableConfig
	catalog     *Catalog
	out         []byte
}

func NewIcebergTable() *IcebergTableServer {
	return &IcebergTableServer{}
}

func (s *IcebergTableServer) Write(ctx context.Context, call IcebergTable_write) error {
	args := call.Args()
	payload, _ := args.Payload()
	s.out = bytes.Clone(payload)
	return nil
}

func (s *IcebergTableServer) Done(ctx context.Context, call IcebergTable_done) error {
	res, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "table: alloc results failed", err))
	}

	if len(s.out) > 0 {
		if err := res.SetOut(s.out); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "table: set out failed", err))
		}
	}

	s.out = nil
	return nil
}

type IcebergScanServer struct{}

func NewIcebergScan() *IcebergScanServer {
	return &IcebergScanServer{}
}

func (s *IcebergScanServer) Write(ctx context.Context, call IcebergScan_write) error {
	return nil
}

func (s *IcebergScanServer) Done(ctx context.Context, call IcebergScan_done) error {
	res, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "scan: alloc results failed", err))
	}

	if err := res.SetOut([]byte{}); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "scan: set out failed", err))
	}

	return nil
}
