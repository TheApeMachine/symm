package main

import (
	"context"
	"fmt"
	"os"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func main() {
	errnie.Apply(&errnie.Config{
		Level: "debug",
	})

	program, err := compiler.CompileFile(
		"manifest/live_level3.json", nil, compiler.DefaultRepository(),
	)
	if err != nil {
		fmt.Printf("Compile failed: %v\n", err)
		os.Exit(1)
	}
	defer program.Release()

	// Seed with BTC/USD
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}
	symbols, err := transport.NewFan_write_Params(segment)
	if err != nil {
		panic(err)
	}
	symbols.SetData([]byte(`["BTC/USD"]`))

	symNodeID, ok := program.NodeMap["symbols"]
	if !ok {
		panic("node symbols not found")
	}

	if err := program.Execute(context.Background(), map[compiler.NodeID]capnp.Struct{symNodeID: capnp.Struct(symbols)}); err != nil {
		fmt.Printf("Execute with symbols failed: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stepCount := 0
	for ctx.Err() == nil {
		if err := program.Execute(ctx, nil); err != nil {
			fmt.Printf("Execute step failed: %v\n", err)
			break
		}

		if res, ok := program.Result("records"); ok && res.IsValid() {
			iterRes := data.Iterate_done_Results(res)
			out, _ := iterRes.Out()
			if len(out) > 0 {
				fmt.Printf("[RECORDS OUT]: %s\n", string(out))
			}
		}
		if res, ok := program.Result("shard_0.books"); ok && res.IsValid() {
			filtRes := data.Filtered(res)
			if filtRes.Which() == data.Filtered_Which_out {
				out, _ := filtRes.Out()
				fmt.Printf("[SHARD_0 BOOKS]: %s\n", string(out))
			}
		}
		if res, ok := program.Result("shard_0.subscription"); ok && res.IsValid() {
			subRes := transport.Fan_done_Results(res)
			out, _ := subRes.Out()
			if len(out) > 0 {
				fmt.Printf("[SHARD_0 SUB OUT]: %s\n", string(out))
			}
		}
		stepCount++
		if stepCount%20 == 0 {
			fmt.Printf("--- Step %d ---\n", stepCount)
			for nodeID := range program.NodeMap {
				if res, ok := program.Result(string(nodeID)); ok && res.IsValid() {
					fmt.Printf("  ran: %s\n", nodeID)
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}
