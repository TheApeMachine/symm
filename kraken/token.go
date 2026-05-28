package kraken

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/symm/kraken/core"
)

/*
Token is the authentication method for the Kraken WebSocket connection.

Kraken returns `expires` as a lifetime in seconds, not an absolute unix
timestamp. To support a meaningful Expired() check the token records the
local time at which the response was received and tests "now ≥ issuedAt +
expires - skew" so reconnect logic can refresh just before the server
rejects the existing token.
*/
type Token struct {
	mu       sync.Mutex
	issuedAt time.Time
	Error    []any `json:"error"`
	Result   struct {
		Token   string `json:"token"`
		Expires int    `json:"expires"`
	} `json:"result"`
}

// expirySkew is the safety margin subtracted from the server-supplied
// lifetime so a refresh is initiated before the existing token can be
// rejected mid-send.
const expirySkew = 30 * time.Second

func NewToken(publicKey, privateKey string) (*Token, error) {
	path := core.WebSocketToken
	nonce := fmt.Sprint(time.Now().UnixMilli())
	bodyMap := map[string]any{"nonce": nonce}

	bodyBytes, err := json.Marshal(bodyMap)

	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	bodyString := string(bodyBytes)
	signature, err := getSignature(privateKey, bodyString, nonce, path)

	if err != nil {
		return nil, fmt.Errorf("get signature: %w", err)
	}

	response, err := client.Post(strings.Join(
		[]string{core.KRAKEN_API_URL, path}, "",
	), client.Config{
		Body: bodyBytes,
		Header: map[string]string{
			"Content-Type": "application/json",
			"API-Key":      publicKey,
			"API-Sign":     signature,
		},
	})

	if err != nil {
		return nil, err
	}

	defer response.Close()

	body := response.Body()

	var token Token

	if err = json.Unmarshal(body, &token); err != nil {
		return nil, err
	}

	if len(token.Error) > 0 {
		return nil, fmt.Errorf("kraken API error: %v", token.Error)
	}

	token.issuedAt = time.Now()

	return &token, nil
}

func (t *Token) Value() string {
	if t == nil {
		return ""
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	return t.Result.Token
}

/*
Expired reports whether the token is past its server-declared lifetime,
minus a safety skew. A token whose issuedAt is zero (constructed by tests
or by direct field assignment without NewToken) is treated as fresh for
the declared lifetime starting now, since we have no other reference.
*/
func (t *Token) Expired() bool {
	if t == nil {
		return true
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Result.Token == "" {
		return true
	}

	if t.Result.Expires <= 0 {
		return false
	}

	issued := t.issuedAt

	if issued.IsZero() {
		return false
	}

	deadline := issued.Add(time.Duration(t.Result.Expires)*time.Second - expirySkew)

	return time.Now().After(deadline)
}

/*
Refresh replaces this token's value and resets issuedAt to now. Returns
nil on success; on failure returns the underlying error from the
GetWebSocketsToken REST call (network failure, signing failure, or a
non-empty Kraken error array). The token's value is left unchanged on
error so callers can continue using the previous token until its real
expiry.
*/
func (t *Token) Refresh(publicKey, privateKey string) error {
	fresh, err := NewToken(publicKey, privateKey)

	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.Result.Token = fresh.Result.Token
	t.Result.Expires = fresh.Result.Expires
	t.issuedAt = fresh.issuedAt

	return nil
}

func getSignature(privateKey, data, nonce, path string) (string, error) {
	message := sha256.New()
	message.Write([]byte(nonce + data))
	return sign(privateKey, []byte(path+string(message.Sum(nil))))
}

func sign(privateKey string, message []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return "", err
	}

	hmacHash := hmac.New(sha512.New, key)
	hmacHash.Write(message)
	return base64.StdEncoding.EncodeToString(hmacHash.Sum(nil)), nil
}
