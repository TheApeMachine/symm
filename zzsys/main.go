package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/compiler"
	nethttp "github.com/theapemachine/symm/nomagique/network/http"
	"github.com/theapemachine/symm/nomagique/ui"
)

/* observer counts the actual UI receiver calls made by the shipping graph. */
type observer struct {
	*nethttp.HTTPServerServer
	mutex  sync.Mutex
	frames int
	seen   map[string]string
	counts map[string]int
}

func (server *observer) Publish(ctx context.Context, call ui.Receiver_publish) error {
	if err := server.HTTPServerServer.Publish(ctx, call); err != nil {
		return err
	}
	encoded, err := call.Args().Data()
	if err != nil {
		return err
	}
	message, err := capnp.Unmarshal(encoded)
	if err != nil {
		return err
	}
	defer message.Release()
	root, err := ui.ReadRootBindings(message)
	if err != nil {
		return err
	}
	values, err := root.Values()
	if err != nil {
		return err
	}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.frames++
	for index := range values.Len() {
		binding := values.At(index)
		component, err := binding.Component()
		if err != nil {
			return err
		}
		value, err := binding.Value()
		if err != nil {
			return err
		}
		server.seen[component] = value
		server.counts[component]++
	}
	return nil
}

func main() {
	monitored := &observer{HTTPServerServer: nethttp.NewHTTPServer(context.Background()), seen: map[string]string{}, counts: map[string]int{}}
	registry := compiler.NewRegistry()
	compiler.RegisterGeneratedPrimitives(registry)
	registry.Register("http.HTTPServer", compiler.Factory{InterfaceID: nethttp.HTTPServer_TypeID, New: func(context.Context, []byte) (capnp.Client, error) {
		return capnp.Client(nethttp.HTTPServer_ServerToClient(monitored)), nil
	}})
	program, err := compiler.CompileFile("manifest/system.json", registry, compiler.DefaultRepository())
	if err != nil {
		fmt.Println("ERR", err)
		return
	}
	defer program.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	err = program.Start(ctx)
	fmt.Println("start returned", err)
	if err := program.Flush(context.WithoutCancel(ctx)); err != nil {
		fmt.Println("flush returned", err)
	}
	monitored.mutex.Lock()
	defer monitored.mutex.Unlock()
	fmt.Println("frames", monitored.frames)
	for key, value := range monitored.seen {
		fmt.Printf("%-24s %6d  %.60s\n", key, monitored.counts[key], value)
	}
}
