package cognition

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/theapemachine/errnie"
)

type WeightServer struct {
	count     uint64
	mass      uint64
	writeStep uint64
}

func NewWeight() *WeightServer {
	return &WeightServer{}
}

func (s *WeightServer) Write(ctx context.Context, call Weight_write) error {
	record, _ := call.Args().Record()
	var pw [3]uint64
	if len(record) >= 24 {
		if err := binary.Read(bytes.NewReader(record), binary.LittleEndian, &pw); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "failed to decode weight", err))
		}
	}

	s.count = pw[0]
	s.mass = pw[1]
	s.writeStep = pw[2]
	return nil
}

func (s *WeightServer) Done(ctx context.Context, call Weight_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetCount(s.count)
	results.SetMass(s.mass)
	results.SetWriteStep(s.writeStep)

	s.count = 0
	s.mass = 0
	s.writeStep = 0
	return nil
}
