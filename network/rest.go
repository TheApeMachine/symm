package network

import (
	"context"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type RestClient struct {
	*runtime.System
	conn *client.Client
}

func NewRestClient(ctx context.Context) *RestClient {
	client := &RestClient{
		conn: client.New(),
	}

	client.System = runtime.NewSystem(ctx, "rest", client)
	return client
}

func (rest *RestClient) Post(
	path string, message []byte, cfg client.Config,
) ([]byte, error) {
	response, err := rest.conn.Post(path, cfg)

	if err != nil {
		return nil, rest.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[symm] websocket rest: unable to connect to server",
			err,
		))
	}

	return response.Body(), nil
}
