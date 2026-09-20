package cognition

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type BasinKeyServer struct {
	Downstream func(context.Context, []byte) error
}

func (s *BasinKeyServer) Write(ctx context.Context, call BasinKey_write) error {
	class, _ := call.Args().Class()
	contextBytes, _ := call.Args().ContextBytes()
	buf := make([]byte, 2+len(contextBytes)+1+len(class))
	buf[0] = 'b'
	buf[1] = '/'
	copy(buf[2:], contextBytes)
	buf[2+len(contextBytes)] = '/'
	copy(buf[3+len(contextBytes):], class)
	return s.Downstream(ctx, buf)
}

func (s *BasinKeyServer) Done(ctx context.Context, call BasinKey_done) error {
	return nil
}



type BasinKeyNode types.StreamNode[any, any]

func NewBasinKey() BasinKeyNode {
	server := &BasinKeyServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
