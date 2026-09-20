package execution

import (
	"context"
	"sync"
)

type GateServer struct {
	holding bool
	mu      sync.Mutex
	Downstream func(context.Context, string) error
}

func NewGateServer(initialHolding bool) *GateServer {
	return &GateServer{holding: initialHolding}
}

func (s *GateServer) Write(ctx context.Context, call Gate_write) error {
	args, err := call.Args().Gate()
	if err != nil {
		return err
	}
	
	action, err := args.Action()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := "wait"
	if action == "enter" {
		if !s.holding {
			s.holding = true
			result = "enter"
		}
	} else if action == "exit" {
		if s.holding {
			s.holding = false
			result = "exit"
		}
	}

	if s.Downstream != nil {
		return s.Downstream(ctx, result)
	}
	return nil
}

func (s *GateServer) Done(ctx context.Context, call Gate_done) error {
	return nil
}
