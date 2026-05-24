package rest

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
)

func signRequest(privateKey, data, nonce, path string) (string, error) {
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
