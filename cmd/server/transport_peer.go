// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net"
	"net/http"
)

type transportPeerKey struct{}

// Capture before RealIP: forwarding headers must never authorize tunnel removal.
func captureTransportPeer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), transportPeerKey{}, r.RemoteAddr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func transportPeerIP(r *http.Request) net.IP {
	address, ok := r.Context().Value(transportPeerKey{}).(string)
	if !ok {
		address = r.RemoteAddr
	}
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		address = host
	}
	return net.ParseIP(address)
}
