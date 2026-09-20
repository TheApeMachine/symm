package cognition

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type SensoryKeyServer struct {
	Downstream func(context.Context, []byte) error
}

func (s *SensoryKeyServer) Write(ctx context.Context, call SensoryKey_write) error {
	contextBytes, _ := call.Args().ContextBytes()
	buf := make([]byte, 2+len(contextBytes))
	buf[0] = 's'
	buf[1] = '/'
	copy(buf[2:], contextBytes)
	return s.Downstream(ctx, buf)
}

func (s *SensoryKeyServer) Done(ctx context.Context, call SensoryKey_done) error {
	return nil
}



type SensoryKeyNode types.StreamNode[any, any]

func NewSensoryKey() SensoryKeyNode {
	server := &SensoryKeyServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
