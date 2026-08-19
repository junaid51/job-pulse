package api

import "net/http"

// deviceID identifies the calling device — identity without accounts. The app
// generates a UUID once and sends it on every request; everything a device
// creates belongs to that id and is invisible to other devices. There is
// nothing to guess worth guessing (a device id only unlocks that device's own
// search profiles) and nothing to log in to.
//
// An absent header is the ” namespace: pre-migration rows and bare curl both
// land there, which keeps local debugging one flag shorter.
func deviceID(r *http.Request) string {
	id := r.Header.Get("X-Device")
	if len(id) > 64 {
		id = id[:64]
	}
	return id
}
