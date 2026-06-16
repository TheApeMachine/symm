package public

import (
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/singleflight"
)

var (
	getResponseCache  sync.Map
	getResponseFlight singleflight.Group
	getCacheTTL       atomic.Int64
)

type cachedGetBody struct {
	body      []byte
	expiresAt time.Time
}

func getRequestCacheKey(endpoint EndpointType, request fiber.Map) string {
	params := url.Values{}

	for key, value := range request {
		params.Add(key, fmt.Sprintf("%v", value))
	}

	return string(endpoint) + "?" + params.Encode()
}

func loadCachedGetBody(cacheKey string) ([]byte, bool) {
	rawEntry, ok := getResponseCache.Load(cacheKey)

	if !ok {
		return nil, false
	}

	entry, entryOK := rawEntry.(cachedGetBody)

	if !entryOK || time.Now().After(entry.expiresAt) {
		getResponseCache.Delete(cacheKey)
		return nil, false
	}

	return entry.body, true
}

func storeCachedGetBody(cacheKey string, body []byte, cacheTTL time.Duration) {
	if cacheTTL <= 0 || len(body) == 0 {
		return
	}

	getResponseCache.Store(cacheKey, cachedGetBody{
		body:      append([]byte(nil), body...),
		expiresAt: time.Now().Add(cacheTTL),
	})
}

func cacheableGetBody(body []byte) bool {
	envelope := restEnvelope{}

	if err := sonic.Unmarshal(body, &envelope); err != nil {
		return false
	}

	if len(envelope.Error) > 0 {
		return false
	}

	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return false
	}

	return true
}

func resetGetCache() {
	getResponseCache.Range(func(key, _ any) bool {
		getResponseCache.Delete(key)
		return true
	})

	getCacheTTL.Store(0)
}
