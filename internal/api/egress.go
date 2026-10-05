package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// echoServices answer with the caller's address and nothing else. Two, so a
// sample can show whether the address depends on where a request is going.
var echoServices = []string{
	"https://api.ipify.org",
	"https://checkip.amazonaws.com",
}

// egress reports the address this server's requests leave from, sampled over
// fresh connections. It exists for Careerjet, whose key works only from
// declared addresses: the host gives no fixed outbound IP, and this is the
// one place that says which address to declare without waiting for a refusal.
func egress() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !tokenGuard(w, r) {
			return
		}
		samples := intParam(r, "samples", 10, 40)
		// No keep-alives: a reused connection would report the same address
		// however the host assigns them, and the question is how it assigns.
		client := &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{DisableKeepAlives: true},
		}
		counts := map[string]map[string]int{}
		for i := 0; i < samples; i++ {
			service := echoServices[i%len(echoServices)]
			address := whoAmI(r.Context(), client, service)
			if counts[address] == nil {
				counts[address] = map[string]int{}
			}
			counts[address][service]++
		}
		writeJSON(w, http.StatusOK, map[string]any{"samples": samples, "addresses": counts})
	}
}

// whoAmI asks one echo service for the caller's address, and on failure says
// why in place of an address, so a sample is never silently short.
func whoAmI(ctx context.Context, client *http.Client, service string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, service, nil)
	if err != nil {
		return "error: " + err.Error()
	}
	resp, err := client.Do(req)
	if err != nil {
		return "error: " + err.Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "error: " + err.Error()
	}
	address := strings.TrimSpace(string(body))
	if net.ParseIP(address) == nil {
		return "error: not an address: " + address
	}
	return address
}
