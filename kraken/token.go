package kraken

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/symm/kraken/core"
)

/*
Token is the authentication method for the Kraken
WebSocket connection.
*/
type Token struct {
	Error  []any `json:"error"`
	Result struct {
		Token   string `json:"token"`
		Expires int    `json:"expires"`
	} `json:"result"`
}

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

	return &token, nil
}

func (t *Token) Value() string {
	if t == nil {
		return ""
	}
	return t.Result.Token
}

func (t *Token) Expired() bool {
	if t == nil || t.Result.Token == "" {
		return true
	}
	return time.Now().Unix() >= int64(t.Result.Expires)
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
