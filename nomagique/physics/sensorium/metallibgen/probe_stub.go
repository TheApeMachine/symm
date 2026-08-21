//go:build !darwin

package main

func metallibLoadError(payload []byte) string {
	_ = payload
	return ""
}
