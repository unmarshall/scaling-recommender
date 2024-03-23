package util

func EmptyOr(val string, defaultVal string) string {
	if val == "" {
		return defaultVal
	}
	return val
}
