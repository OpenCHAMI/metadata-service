// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	v1 "github.com/OpenCHAMI/cloud-init/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/cloud-init/internal/storage"
	"github.com/OpenCHAMI/cloud-init/pkg/smdclient"
	"github.com/OpenCHAMI/cloud-init/pkg/wireguard"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openchami/fabrica/pkg/events"
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

// wgPeerNamespace is the UUID v5 namespace used to derive deterministic peer UIDs from public keys.
var wgPeerNamespace = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

// peerUIDForKey derives a deterministic resource UID from a WireGuard public key.
// This makes /wg-init idempotent: the same keypair always maps to the same resource.
func peerUIDForKey(publicKey string) string {
	return uuid.NewSHA1(wgPeerNamespace, []byte("wireguardpeer."+publicKey)).String()
}

func resolvePhoneHomePeerUID(ctx context.Context, routeID, clientIP string) (string, error) {
	if strings.TrimSpace(routeID) != "" {
		if _, err := storage.LoadWireGuardPeer(ctx, routeID); err == nil {
			return routeID, nil
		}
	}

	peers, err := storage.LoadAllWireGuardPeers(ctx)
	if err != nil {
		return "", err
	}

	for _, peer := range peers {
		if peer == nil {
			continue
		}
		if strings.TrimSpace(peer.Metadata.Name) == clientIP {
			return peer.Metadata.UID, nil
		}
	}

	return "", fmt.Errorf("no WireGuardPeer found for route id %q or client IP %q", routeID, clientIP)
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
		peerUID := peerUIDForKey(req.PublicKey)

		// Idempotent: if this keypair is already registered, return the existing allocation.
		if existingVPNIP, ok := controller.GetPeerVPNIP(peerUID); ok {
			if smd != nil {
				if id, err := smd.IDfromIP(clientIP); err == nil {
					_ = smd.AddWGIP(id, existingVPNIP)
				}
			}
			resp := wgInitResponse{
				Message:      "WireGuard tunnel already active",
				ClientVPNIP:  existingVPNIP,
				ServerPubKey: controller.PublicKey(),
				ServerIP:     controller.ServerIPString(),
				ServerPort:   fmt.Sprintf("%d", controller.ListenPort()),
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Allocate a VPN IP from the pool.
		netIP, err := controller.IPAllocator.NextAvailable()
		if err != nil {
			http.Error(w, "no VPN IPs available", http.StatusServiceUnavailable)
			return
		}
		vpnIP := netIP.IP.String()
		allowedIP := vpnIP + "/32"

		// Configure the device synchronously so we can return the VPN IP immediately.
		// The reconciler will call UpsertPeer again on the create event — idempotent no-op.
		if err := controller.UpsertPeer(peerUID, req.PublicKey, allowedIP); err != nil {
			_ = controller.IPAllocator.Release(netIP)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Persist as a WireGuardPeer resource so the reconciler manages its lifecycle.
		peer := &v1.WireGuardPeer{}
		peer.APIVersion = "cloud-init.openchami.io/v1"
		peer.Kind = "WireGuardPeer"
		peer.Metadata.UID = peerUID
		peer.Metadata.Name = clientIP
		peer.Spec = v1.WireGuardPeerSpec{
			PublicKey:   req.PublicKey,
			AllowedIP:   allowedIP,
			Description: "created via /wg-init from " + clientIP,
		}
		if saveErr := storage.SaveWireGuardPeer(r.Context(), peer); saveErr != nil {
			fmt.Printf("Warning: failed to persist WireGuardPeer resource %s: %v\n", peerUID, saveErr)
		} else {
			_ = events.PublishResourceCreated(r.Context(), "WireGuardPeer", peerUID, peer.Metadata.Name, peer)
		}

		if smd != nil {
			if id, err := smd.IDfromIP(clientIP); err == nil {
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
		routeID := chi.URLParam(r, "id")
		peerUID, err := resolvePhoneHomePeerUID(r.Context(), routeID, clientIP)
		if err != nil {
			fmt.Printf("Warning: phone-home peer lookup failed (id=%q clientIP=%q): %v\n", routeID, clientIP, err)
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := controller.RemovePeerByID(peerUID); err != nil {
			fmt.Printf("Warning: failed removing peer %q from phone-home: %v\n", peerUID, err)
		}
		w.WriteHeader(http.StatusOK)
	})
}
