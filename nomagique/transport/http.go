package transport

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
HTTPRequest performs an HTTP request using the configured method and URL ports,
processing input data and returning the parsed JSON response map.
*/
type HTTPRequest types.Value[map[string]any, map[string]any]

/*
NewHTTPRequest constructs a closure that executes an HTTP request.
method and rawURL are types.String port closures (either static constants or dynamic wires).
*/
func NewHTTPRequest(method types.String, rawURL types.String) HTTPRequest {
	cc := client.New()

	return func(body map[string]any) map[string]any {
		m := "GET"
		if method != nil {
			if evaluated := method(body); evaluated != "" {
				m = strings.ToUpper(evaluated)
			}
		}

		u := ""
		if rawURL != nil {
			u = rawURL(body)
		}

		cfg := client.Config{
			Ctx: context.Background(),
			Header: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "application/json",
			},
		}

		if headers, ok := body["headers"].(map[string]string); ok {
			for k, v := range headers {
				cfg.Header[k] = v
			}
		}

		if params, ok := body["params"].(map[string]string); ok {
			cfg.Param = params
		}

		if bodyPayload, ok := body["body"]; ok {
			cfg.Body = bodyPayload
		} else if len(body) > 0 && m != "GET" && m != "HEAD" {
			cfg.Body = body
		}

		var resp *client.Response
		var err error

		switch m {
		case "POST":
			resp, err = cc.Post(u, cfg)
		case "PUT":
			resp, err = cc.Put(u, cfg)
		case "PATCH":
			resp, err = cc.Patch(u, cfg)
		case "DELETE":
			resp, err = cc.Delete(u, cfg)
		case "HEAD":
			resp, err = cc.Head(u, cfg)
		case "GET":
			fallthrough
		default:
			resp, err = cc.Get(u, cfg)
		}

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[transport] HTTP request execution failed",
				err,
			))
			return map[string]any{"error": err.Error()}
		}

		out := make(map[string]any)
		if len(resp.Body()) > 0 {
			if unmarshalErr := sonic.Unmarshal(resp.Body(), &out); unmarshalErr != nil {
				out["raw"] = string(resp.Body())
			}
		}
		out["status_code"] = resp.StatusCode()

		return out
	}
}
