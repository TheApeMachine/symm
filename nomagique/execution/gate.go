package execution

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
)

type GateServer struct {
	holding bool
	mu      sync.Mutex
	out     string
}

func NewGate() *GateServer {
	return &GateServer{}
}

func (s *GateServer) Write(ctx context.Context, call Gate_write) error {
	action, err := call.Args().In()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "failed to read in arg", err))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := "wait"
	if action == "enter" && !s.holding {
		s.holding = true
		result = "enter"
	}

	if action == "exit" && s.holding {
		s.holding = false
		result = "exit"
	}

	s.out = result
	return nil
}

func (s *GateServer) Done(ctx context.Context, call Gate_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = ""
	return nil
}
