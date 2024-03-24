package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"unmarshall/scaling-recommender/internal/scaler"
)

func Log(w scaler.LogWriterFlusher, msg string) {
	_, _ = fmt.Fprintf(w, "[ %s ] : %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
	w.(http.Flusher).Flush()
}

func Logf(w scaler.LogWriterFlusher, format string, a ...any) {
	msg := fmt.Sprintf(format, a)
	Log(w, msg)
}

func InternalError(w scaler.LogWriterFlusher, err error) {
	http.Error(w.(http.ResponseWriter), err.Error(), http.StatusInternalServerError)
}

func GetIntQueryParam(r *http.Request, name string, defVal int) int {
	valStr := r.URL.Query().Get(name)
	if valStr == "" {
		return defVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		slog.Error("cannot convert to int, using default", "name", name, "value", valStr, "default", defVal)
		return defVal
	}
	return val
}
