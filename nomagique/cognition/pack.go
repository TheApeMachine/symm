package cognition

import (
	"bytes"
	"context"
	"encoding/binary"
)

type PackServer struct {
	Downstream func(context.Context, []byte) error
}

func (s *PackServer) Write(ctx context.Context, call Pack_write) error {
	var pw [3]uint64
	pw[0] = call.Args().Count()
	pw[1] = call.Args().Mass()
	pw[2] = call.Args().WriteStep()
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, pw)
	return s.Downstream(ctx, buf.Bytes())
}

func (s *PackServer) Done(ctx context.Context, call Pack_done) error {
	return nil
}
