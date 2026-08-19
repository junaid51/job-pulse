package api

import "net/http"

// cors lets browsers call the API from another origin. The web build is served
// from its own dev port, so every request it makes is cross-origin; without
// these headers Chrome blocks all of them. There is no auth and no cookies, so
// a blanket allow gives away nothing that binding to localhost has not already
// decided.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			// The preflight: answer it here so handlers never see it.
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
