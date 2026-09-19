package transport

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Nonce generates a monotonic int64 sequence.
*/
type Nonce types.Value[any, int64]

func NewNonce() Nonce {
	var counter int64 = time.Now().UnixNano()

	return func(any) int64 {
		return atomic.AddInt64(&counter, 1)
	}
}

/*
Timestamp produces current Unix epoch in milliseconds.
*/
type Timestamp types.Value[any, int64]

func NewTimestamp() Timestamp {
	return func(any) int64 {
		return time.Now().UnixMilli()
	}
}

/*
SHA256 computes a SHA-256 hash over bytes.
*/
type SHA256 types.Value[[]byte, []byte]

func NewSHA256() SHA256 {
	return func(data []byte) []byte {
		h := sha256.Sum256(data)
		return h[:]
	}
}

/*
HMACSHA512 signs message bytes using the secret provided via the secret port.
*/
type HMACSHA512 types.Value[[]byte, []byte]

func NewHMACSHA512(secret types.Bytes) HMACSHA512 {
	return func(message []byte) []byte {
		var sec []byte
		if secret != nil {
			sec = secret(message)
		}
		mac := hmac.New(sha512.New, sec)
		mac.Write(message)
		return mac.Sum(nil)
	}
}

/*
HMACSHA256 signs message bytes using the secret provided via the secret port.
*/
type HMACSHA256 types.Value[[]byte, []byte]

func NewHMACSHA256(secret types.Bytes) HMACSHA256 {
	return func(message []byte) []byte {
		var sec []byte
		if secret != nil {
			sec = secret(message)
		}
		mac := hmac.New(sha256.New, sec)
		mac.Write(message)
		return mac.Sum(nil)
	}
}

/*
Base64Encode encodes bytes into a base64 string.
*/
type Base64Encode types.Value[[]byte, string]

func NewBase64Encode() Base64Encode {
	return func(data []byte) string {
		return base64.StdEncoding.EncodeToString(data)
	}
}

/*
Base64Decode decodes a base64 string into bytes.
*/
type Base64Decode types.Value[string, []byte]

func NewBase64Decode() Base64Decode {
	return func(data string) []byte {
		decoded, _ := base64.StdEncoding.DecodeString(data)
		return decoded
	}
}

/*
HeaderAuth attaches a key-value header to the data map using key and value ports.
*/
type HeaderAuth types.Value[map[string]any, map[string]any]

func NewHeaderAuth(key types.String, value types.String) HeaderAuth {
	return func(in map[string]any) map[string]any {
		if in == nil {
			in = make(map[string]any)
		}

		k := ""
		if key != nil {
			k = key(in)
		}

		v := ""
		if value != nil {
			v = value(in)
		}

		headers, ok := in["headers"].(map[string]string)
		if !ok || headers == nil {
			headers = make(map[string]string)
			in["headers"] = headers
		}

		if k != "" {
			headers[k] = v
		}

		return in
	}
}

/*
BearerAuth attaches a Bearer authorization header using a token port.
*/
type BearerAuth types.Value[map[string]any, map[string]any]

func NewBearerAuth(token types.String) BearerAuth {
	return func(in map[string]any) map[string]any {
		if in == nil {
			in = make(map[string]any)
		}

		t := ""
		if token != nil {
			t = token(in)
		}

		headers, ok := in["headers"].(map[string]string)
		if !ok || headers == nil {
			headers = make(map[string]string)
			in["headers"] = headers
		}

		if t != "" {
			headers["Authorization"] = "Bearer " + t
		}

		return in
	}
}
