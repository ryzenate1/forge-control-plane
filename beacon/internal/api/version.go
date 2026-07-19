package api

import (
	"net/http"
)

const CurrentVersion = "v1"

type VersionedHandler struct {
	Version string
	Handler http.Handler
}

func VersionedRouter(handlers ...VersionedHandler) http.Handler {
	mux := http.NewServeMux()
	for _, vh := range handlers {
		mux.Handle("/"+vh.Version+"/", http.StripPrefix("/"+vh.Version, vh.Handler))
	}
	return mux
}
