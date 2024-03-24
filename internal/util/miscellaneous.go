package util

import (
	"crypto/rand"
	"encoding/hex"
)

func EmptyOr(val string, defaultVal string) string {
	if val == "" {
		return defaultVal
	}
	return val
}

func GenerateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
