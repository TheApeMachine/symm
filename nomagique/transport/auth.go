package transport

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewNonce creates a monotonic nonce generator closure.
*/
func NewNonce() types.Value[any, int64] {
	var counter int64 = time.Now().UnixNano()

	return func(any) int64 {
		return atomic.AddInt64(&counter, 1)
	}
}

/*
NewTimestamp creates a timestamp generator closure returning current Unix epoch in milliseconds.
*/
func NewTimestamp() types.Value[any, int64] {
	return func(any) int64 {
		return time.Now().UnixMilli()
	}
}

/*
NewSHA256 creates a SHA-256 hash closure over bytes.
*/
func NewSHA256() types.Value[[]byte, []byte] {
	return func(data []byte) []byte {
		h := sha256.Sum256(data)
		return h[:]
	}
}

/*
NewHMACSHA512 creates an HMAC-SHA512 signing closure using the secret key.
*/
func NewHMACSHA512(secret []byte) types.Value[[]byte, []byte] {
	return func(message []byte) []byte {
		mac := hmac.New(sha512.New, secret)
		mac.Write(message)
		return mac.Sum(nil)
	}
}

/*
NewHMACSHA256 creates an HMAC-SHA256 signing closure using the secret key.
*/
func NewHMACSHA256(secret []byte) types.Value[[]byte, []byte] {
	return func(message []byte) []byte {
		mac := hmac.New(sha256.New, secret)
		mac.Write(message)
		return mac.Sum(nil)
	}
}

/*
NewSigner creates an HTTPRequest signing closure using API Key, Secret, and optional custom header names.
Optional headers are: keyHeader, signHeader, nonceHeader.
Defaults to "API-Key", "API-Sign", "Nonce".
*/
func NewSigner(apiKey, secretBase64 string, headers ...string) types.Value[*HTTPRequest, *HTTPRequest] {
	keyHeader := "API-Key"
	signHeader := "API-Sign"
	nonceHeader := "Nonce"

	if len(headers) > 0 && headers[0] != "" {
		keyHeader = headers[0]
	}

	if len(headers) > 1 && headers[1] != "" {
		signHeader = headers[1]
	}

	if len(headers) > 2 && headers[2] != "" {
		nonceHeader = headers[2]
	}

	decodedSecret, _ := base64.StdEncoding.DecodeString(secretBase64)
	if len(decodedSecret) == 0 {
		decodedSecret = []byte(secretBase64)
	}

	signer := NewHMACSHA512(decodedSecret)

	return func(req *HTTPRequest) *HTTPRequest {
		if req == nil {
			return nil
		}

		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}

		nonce := time.Now().UnixNano()
		payload := fmt.Sprintf("%d%s", nonce, string(req.Body))
		signature := signer([]byte(payload))

		req.Headers[keyHeader] = apiKey
		req.Headers[signHeader] = base64.StdEncoding.EncodeToString(signature)
		req.Headers[nonceHeader] = fmt.Sprintf("%d", nonce)

		return req
	}
}

/*
NewBearerAuth attaches a Bearer token authorization header to an HTTPRequest.
*/
func NewBearerAuth(token string) types.Value[*HTTPRequest, *HTTPRequest] {
	return func(req *HTTPRequest) *HTTPRequest {
		if req == nil {
			return nil
		}

		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}

		req.Headers["Authorization"] = "Bearer " + token
		return req
	}
}

/*
NewHeaderAuth attaches a generic key-value header to an HTTPRequest.
*/
func NewHeaderAuth(key, value string) types.Value[*HTTPRequest, *HTTPRequest] {
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
