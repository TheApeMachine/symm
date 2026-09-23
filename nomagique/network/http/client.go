package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/theapemachine/errnie"
)

/* HTTPClientServer performs one graph-specified HTTP exchange per evaluation. */
type HTTPClientServer struct {
	client http.Client
	out    []byte
	status uint16
}

/* NewHTTPClient constructs an idle request primitive. */
func NewHTTPClient() *HTTPClientServer { return &HTTPClientServer{} }

/* Write sends the declared request. Non-success responses remain explicit errors. */
func (server *HTTPClientServer) Write(ctx context.Context, call HTTPClient_write) error {
	server.out, server.status = nil, 0
	address, err := call.Args().Url()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http client: URL", err))
	}
	method, err := call.Args().Method()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http client: method", err))
	}
	if address == "" || method == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "http client: URL and method are required", nil))
	}
	body, err := call.Args().Body()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http client: body", err))
	}
	request, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http client: request", err))
	}
	headers, err := call.Args().Headers()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "http client: headers", err))
	}
	for index := 0; index < headers.Len(); index++ {
		line, err := headers.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "http client: header", err))
		}
		name, value, found := strings.Cut(string(line), ":")
		name = strings.TrimSpace(name)

		if !found || name == "" {
			return errnie.Error(errnie.Err(errnie.Validation, "http client: header is not a \"Name: value\" line", nil))
		}
		request.Header.Add(name, strings.TrimSpace(value))
	}
	response, err := server.client.Do(request)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "http client: exchange", err))
	}
	payload, err := io.ReadAll(response.Body)
	err = errors.Join(err, response.Body.Close())
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "http client: response body", err))
	}
	server.status = uint16(response.StatusCode)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return errnie.Error(errnie.Err(errnie.BadGateway, "http client: "+response.Status, nil))
	}
	server.out = payload
	return nil
}

/* Done publishes the response and clears the evaluation. */
func (server *HTTPClientServer) Done(ctx context.Context, call HTTPClient_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "http client: results", err))
	}
	result.SetStatus(server.status)
	if err := result.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "http client: output", err))
	}
	server.out, server.status = nil, 0
	return nil
}
