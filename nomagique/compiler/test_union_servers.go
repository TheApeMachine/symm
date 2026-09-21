package compiler

import (
	"context"
	"sync"
)

type TestUnionServer struct {
	mu        sync.Mutex
	val       float64
	chooseYes bool
}

func NewTestUnionServer(chooseYes bool) *TestUnionServer {
	return &TestUnionServer{chooseYes: chooseYes}
}

func (s *TestUnionServer) Write(ctx context.Context, call TestUnion_write) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.val = call.Args().In()
	return nil
}

func (s *TestUnionServer) Done(ctx context.Context, call TestUnion_done) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := call.AllocResults()
	if err != nil {
		return err
	}
	if s.chooseYes {
		res.SetYes(s.val)
	} else {
		res.SetNo()
	}
	return nil
}

type TestSinkVoidServer struct {
	mu     sync.Mutex
	called bool
}

func NewTestSinkVoidServer() *TestSinkVoidServer {
	return &TestSinkVoidServer{}
}

func (s *TestSinkVoidServer) Write(ctx context.Context, call TestSinkVoid_write) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called = true
	return nil
}

func (s *TestSinkVoidServer) Done(ctx context.Context, call TestSinkVoid_done) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := call.AllocResults()
	if err != nil {
		return err
	}
	res.SetOk(s.called)
	return nil
}
