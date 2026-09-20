package store

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

// GridServer implements Grid_Server from the capnp schema.
// It acts as a virtual lock-free metric coordinate store over Cap'n Proto.
type GridServer struct {
	cells []Grid
	coords [][2]int
}

func NewGridServer() *GridServer {
	return &GridServer{
		cells:  make([]Grid, 0),
		coords: make([][2]int, 0),
	}
}

func (s *GridServer) Poke(ctx context.Context, call Grid_poke) error {
	payload, err := call.Args().Payload()
	if err != nil {
		return err
	}

	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	outList, err := res.NewCells(int32(len(s.cells)))
	if err != nil {
		return err
	}

	for i, cell := range s.cells {
		if cell.IsValid() {
			pokeCall, release := cell.Poke(ctx, func(p Grid_poke_Params) error {
				p.SetPayload(payload)
				return nil
			})
			
			pokeRes, err := pokeCall.Struct()
			if err == nil {
				cellsRes, err := pokeRes.Cells()
				if err == nil && cellsRes.Len() > 0 {
					outList.Set(i, cellsRes.At(0))
				}
			}
			release()
		}
	}

	return nil
}

func (s *GridServer) Peek(ctx context.Context, call Grid_peek) error {
	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	outList, err := res.NewCells(int32(len(s.cells)))
	if err != nil {
		return err
	}

	for i, cell := range s.cells {
		if cell.IsValid() {
			peekCall, release := cell.Peek(ctx, func(p Grid_peek_Params) error {
				return nil
			})
			
			peekRes, err := peekCall.Struct()
			if err == nil {
				cellsRes, err := peekRes.Cells()
				if err == nil && cellsRes.Len() > 0 {
					outList.Set(i, cellsRes.At(0))
				}
			}
			release()
		}
	}

	return nil
}

func (s *GridServer) Register(ctx context.Context, call Grid_register) error {
	payload, err := call.Args().Payload()
	if err != nil {
		return err
	}

	// In a real capability-based model, the payload might contain the Client/Capability itself,
	// or we register a new local cell that proxies the capability.
	// For now, we will decode the payload as a Grid capability and store it.
	
	if payload.IsValid() {
		// Allocate unique 2D coordinates dynamically using square expansion
		n := len(s.cells)
		sz := int(math.Sqrt(float64(n)))
		rem := n - sz*sz
		x, y := 0, 0

		if rem < sz {
			x, y = rem, sz
		} else {
			x, y = sz, rem-sz
		}

		s.coords = append(s.coords, [2]int{x, y})
		
		// Attempt to extract a client capability from the payload if it's an interface pointer.
		client := payload.Interface().Client()
		if client.IsValid() {
			s.cells = append(s.cells, Grid(client))
		}
	}

	return nil
}



type GridNode types.StreamNode[any, any]

func NewGrid() GridNode {
	server := &GridServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
