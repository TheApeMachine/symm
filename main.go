package main

import (
	"context"
	"fmt"

	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/nomagique/arithmetic"
)

func main() {
	cmd.Execute()
	add := arithmetic.NewAdd()
	client := arithmetic.Add_ServerToClient(add)
	ctx := context.Background()

	err := client.Write(ctx, func(p arithmetic.Add_write_Params) error {
		p.SetA(1)
		p.SetB(2)
		return nil
	})

	if err != nil {
		fmt.Errorf("write failed: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		fmt.Errorf("streaming failed: %w", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()

	res, err := future.Struct()
	if err != nil {
		fmt.Errorf("done failed: %w", err)
	}

	fmt.Println("result:", res.Out())
}
