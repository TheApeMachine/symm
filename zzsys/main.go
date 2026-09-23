package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/ui"
)

func main() {
	program, err := compiler.CompileFile("manifest/system.json", nil, compiler.DefaultRepository())
	if err != nil {
		fmt.Println("ERR", err)
		return
	}
	defer program.Release()
	var mu sync.Mutex
	frames := 0
	seen := map[string]string{}
	counts := map[string]int{}
	program.Publish = func(data []byte) {
		msg, err := capnp.Unmarshal(data)
		if err != nil {
			panic(err)
		}
		root, _ := ui.ReadRootBindings(msg)
		values, _ := root.Values()
		mu.Lock()
		defer mu.Unlock()
		frames++
		for i := 0; i < values.Len(); i++ {
			b := values.At(i)
			c, _ := b.Component()
			v, _ := b.Value()
			seen[c] = v
			counts[c]++
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	err = program.Start(ctx)
	fmt.Println("start returned", err)
	mu.Lock()
	fmt.Println("frames", frames)
	for k, v := range seen {
		fmt.Printf("%-24s %6d  %.60s\n", k, counts[k], v)
	}
	mu.Unlock()
}
