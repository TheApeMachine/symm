package cognition

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/theapemachine/errnie"
)

type PackServer struct {
	out []byte
}

func NewPack() *PackServer {
	return &PackServer{}
}

func (s *PackServer) Write(ctx context.Context, call Pack_write) error {
	var pw [3]uint64
	pw[0] = call.Args().Count()
	pw[1] = call.Args().Mass()
	pw[2] = call.Args().WriteStep()

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, pw); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to write binary", err))
	}

	s.out = buf.Bytes()
	return nil
}

func (s *PackServer) Done(ctx context.Context, call Pack_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	if err := results.SetOut(s.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to set out", err))
	}

	s.out = nil
	return nil
}
