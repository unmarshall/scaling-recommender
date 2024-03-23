package simulation

import "net/http"

type HandlerFn func(w http.ResponseWriter, r *http.Request)

type Scenario interface {
	Name() string
	HandlerFn() HandlerFn
}
