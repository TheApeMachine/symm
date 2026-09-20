package cognition

import (
	"bytes"
	"context"
	"encoding/binary"
)

type WeightServer struct {
	Downstream func(context.Context, uint64, uint64, uint64) error
}

func (s *WeightServer) Write(ctx context.Context, call Weight_write) error {
	record, _ := call.Args().Record()
	var pw [3]uint64
	if len(record) >= 24 {
		_ = binary.Read(bytes.NewReader(record), binary.LittleEndian, &pw)
	}
	return s.Downstream(ctx, pw[0], pw[1], pw[2])
}

func (s *WeightServer) Done(ctx context.Context, call Weight_done) error {
	return nil
}
