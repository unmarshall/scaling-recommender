package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samber/lo"
)

func EmptyOr(val string, defaultVal string) string {
	if val == "" {
		return defaultVal
	}
	return val
}

func NilOr[T any](val *T, defaultVal T) T {
	if val == nil {
		return defaultVal
	}
	return *val
}

func GenerateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func AsMap[K comparable, V any](tuple lo.Tuple2[K, V]) map[K]V {
	return map[K]V{tuple.A: tuple.B}
}

func GetAbsoluteConfigPath(filePaths ...string) (*string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	currDirAbsPath, err := filepath.Abs(currentDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for current directory: %w", err)
	}
	configAbsPath := filepath.Join(currDirAbsPath, filepath.Join(filePaths...))
	return &configAbsPath, nil
}
