package transport

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
HTTPRequest holds parameters for constructing and executing an HTTP request.
*/
type HTTPRequest struct {
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]string
	Body        []byte
}

/*
HTTPResponse holds the result of an HTTP execution.
*/
type HTTPResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

/*
NewHTTPRequest constructs a closure that produces an HTTPRequest template.
*/
func NewHTTPRequest(method, rawURL string) types.Value[[]byte, *HTTPRequest] {
	return func(body []byte) *HTTPRequest {
		return &HTTPRequest{
			Method:      method,
			URL:         rawURL,
			Headers:     make(map[string]string),
			QueryParams: make(map[string]string),
			Body:        body,
		}
	}
}

/*
NewHTTPHeader attaches a key-value header to the HTTPRequest.
*/
func NewHTTPHeader(key, value string) types.Value[*HTTPRequest, *HTTPRequest] {
	return func(req *HTTPRequest) *HTTPRequest {
		if req == nil {
			return nil
		}

		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}

		req.Headers[key] = value
		return req
	}
}

/*
NewHTTPParam attaches a query parameter to the HTTPRequest.
*/
func NewHTTPParam(key, value string) types.Value[*HTTPRequest, *HTTPRequest] {
	return func(req *HTTPRequest) *HTTPRequest {
		if req == nil {
			return nil
		}

		if req.QueryParams == nil {
			req.QueryParams = make(map[string]string)
		}

		req.QueryParams[key] = value
		return req
	}
}

/*
NewHTTPExecute performs the HTTP request using the provided http.Client.
*/
func NewHTTPExecute(client *http.Client) types.Value[*HTTPRequest, *HTTPResponse] {
	if client == nil {
		client = http.DefaultClient
	}

	return func(req *HTTPRequest) *HTTPResponse {
		if req == nil {
			return nil
		}

		reqURL, err := url.Parse(req.URL)
		if err != nil {
			errnie.Error(errnie.Err(errnie.Validation, "transport: invalid request URL", err))
			return nil
		}

		if len(req.QueryParams) > 0 {
			q := reqURL.Query()
			for k, v := range req.QueryParams {
				q.Set(k, v)
			}
			reqURL.RawQuery = q.Encode()
		}

		var bodyReader io.Reader
		if len(req.Body) > 0 {
			bodyReader = bytes.NewReader(req.Body)
		}

		httpReq, err := http.NewRequestWithContext(context.Background(), req.Method, reqURL.String(), bodyReader)
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "transport: failed to create http request", err))
			return nil
		}

		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "transport: http execution failed", err))
			return nil
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				errnie.Error(errnie.Err(errnie.IO, "transport: close response body", closeErr))
			}
		}()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "transport: read response body", err))
			return nil
		}

		return &HTTPResponse{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Body:       respBytes,
		}
	}
}

/*
NewResponseExtract returns the byte slice body from an HTTPResponse.
*/
func NewResponseExtract() types.Value[*HTTPResponse, []byte] {
	return func(resp *HTTPResponse) []byte {
		if resp == nil {
			return nil
		}

		return resp.Body
	}
}
