// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
	"github.com/OpenCHAMI/metadata-service/pkg/wireguard"
	"github.com/go-chi/chi/v5"
)

type wgInitRequest struct {
	PublicKey string `json:"public_key"`
}

type wgInitResponse struct {
	Message      string `json:"message"`
	ClientVPNIP  string `json:"client-vpn-ip"`
	ServerPubKey string `json:"server-public-key"`
	ServerIP     string `json:"server-ip"`
	ServerPort   string `json:"server-port"`
}

func getClientIPFromRequest(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		return xff
	}
	fwd := r.Header.Get("Forwarded")
	if fwd != "" {
		parts := strings.Split(fwd, ";")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "for=") {
				return strings.Trim(strings.Split(p, "=")[1], "\"")
			}
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func registerWireGuardRoutes(r chi.Router, controller *wireguard.Controller, smd smdclient.SMDClient) {
	r.Post("/wg-init", func(w http.ResponseWriter, r *http.Request) {
		if controller == nil {
			http.Error(w, "wireguard disabled", http.StatusServiceUnavailable)
			return
		}
		var req wgInitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PublicKey == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		clientIP := getClientIPFromRequest(r)
		vpnIP, err := controller.AddPeer(clientIP, req.PublicKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if smd != nil {
			if id, err := smdclient.ResolveComponentID(smd, clientIP); err == nil {
				_ = smd.AddWGIP(id, vpnIP)
			}
		}
		resp := wgInitResponse{
			Message:      "WireGuard tunnel created successfully",
			ClientVPNIP:  vpnIP,
			ServerPubKey: controller.PublicKey(),
			ServerIP:     controller.ServerIPString(),
			ServerPort:   fmt.Sprintf("%d", controller.ListenPort()),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})

	r.Post("/phone-home/{id}", func(w http.ResponseWriter, r *http.Request) {
		if controller == nil {
			http.Error(w, "wireguard disabled", http.StatusServiceUnavailable)
			return
		}
		clientIP := getClientIPFromRequest(r)
		peerKey := clientIP
		if smd != nil {
			if id, err := smdclient.ResolveComponentID(smd, clientIP); err == nil {
				if ip, err := smd.IPfromID(id); err == nil && ip != "" {
					peerKey = ip
				}
			}
		}
		_ = controller.RemovePeerByID(peerKey)
		w.WriteHeader(http.StatusOK)
	})
}
