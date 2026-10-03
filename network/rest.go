package network

import (
	"fmt"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
)

type RestClient struct {
	conn *client.Client
}

func NewRestClient() *RestClient {
	return &RestClient{
		conn: client.New(),
	}
}

func (rest *RestClient) Post(url string, data []byte) ([]byte, error) {
	response, err := rest.conn.Post(url, client.Config{
		Header: map[string]string{
			"Content-Type": "application/json",
		},
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("unable to create request: %s", url),
			err,
		))
	}

	return response.Body(), nil
}
