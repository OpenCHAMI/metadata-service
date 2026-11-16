// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openchami/fabrica/pkg/events"
	fabricaStorage "github.com/openchami/fabrica/pkg/storage"
	"github.com/openchami/metadata-service/internal/storage"
	"github.com/openchami/metadata-service/pkg/notifications"
	"github.com/openchami/metadata-service/pkg/smdclient"
	"github.com/openchami/metadata-service/pkg/wireguard"
)

const maxPhoneHomeBody = 16 << 10

func registerPhoneHomeRoute(r chi.Router, controller *wireguard.Controller, smd smdclient.SMDClient) {
	r.Post("/phone-home/{id}", func(w http.ResponseWriter, r *http.Request) {
		receipt, status, err := parsePhoneHome(w, r)
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		// Source-address checks always use the transport peer, never client headers.
		ip := transportPeerIP(r)
		if ip != nil {
			receipt.TransportIP = ip.String()
		}
		receipt.IdentityBasis = "reported"
		var peerUID string
		if controller != nil && ip != nil && controller.Network.Contains(ip) {
			peerUID, err = resolvePhoneHomePeerUID(r.Context(), receipt.NodeID, smd)
			if err != nil {
				http.Error(w, "unable to resolve WireGuard peer", http.StatusInternalServerError)
				return
			}
			peer, loadErr := storage.LoadWireGuardPeer(r.Context(), peerUID)
			if loadErr != nil && !errors.Is(loadErr, fabricaStorage.ErrNotFound) {
				http.Error(w, "unable to load WireGuard peer", http.StatusInternalServerError)
				return
			}
			if peer != nil {
				allowedIP, parseErr := vpnIPFromAllowedCIDR(peer.Spec.AllowedIP)
				sourceMatchesIntent := parseErr == nil && ip.Equal(net.ParseIP(allowedIP))
				vpnIP, active := controller.GetPeerVPNIP(peerUID)
				sourceMatchesTunnel := active && ip.Equal(net.ParseIP(vpnIP))
				if !sourceMatchesIntent || !sourceMatchesTunnel {
					http.Error(w, "phone-home does not own WireGuard peer", http.StatusForbidden)
					return
				}
				receipt.IdentityBasis = "wireguard_address"
				receipt.PeerUID = peerUID
			}
		}
		receiptID, emitErr := notifications.Emit(
			r.Context(), notifications.PhoneHomeReceived, "nodes/"+receipt.NodeID, receipt,
		)
		if emitErr != nil {
			log.Printf("Phone-home notification enqueue failed: %v", emitErr)
		}
		// A report received outside the caller's tunnel cannot request peer removal.
		if receipt.PeerUID == "" {
			w.WriteHeader(http.StatusOK)
			return
		}
		peer, err := storage.LoadWireGuardPeer(r.Context(), peerUID)
		if errors.Is(err, fabricaStorage.ErrNotFound) {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err != nil {
			http.Error(w, "unable to load WireGuard peer", http.StatusInternalServerError)
			return
		}
		if !events.AreLifecycleEventsEnabled() || events.GetGlobalEventBus() == nil {
			http.Error(w, "WireGuard shutdown eventing unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := storage.DeleteWireGuardPeer(r.Context(), peerUID); err != nil {
			if !errors.Is(err, fabricaStorage.ErrNotFound) {
				http.Error(w, "unable to delete WireGuard peer intent", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		details := notifications.Shutdown{
			Reason: "phone_home", RequestID: receipt.RequestID, PhoneHomeEventID: receiptID,
		}
		metadata := map[string]interface{}{"phoneHomeID": receipt.NodeID, "shutdown": details}
		if err := events.PublishResourceDeleted(
			r.Context(), "WireGuardPeer", peerUID, peer.Metadata.Name, metadata,
		); err != nil {
			// Failed enqueue must not strand an active tunnel without its intent.
			if restoreErr := storage.SaveWireGuardPeer(r.Context(), peer); restoreErr != nil {
				log.Printf("Failed to restore WireGuard peer intent after enqueue failure: %v", restoreErr)
			}
			http.Error(w, "unable to queue WireGuard shutdown; retry phone-home", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func parsePhoneHome(w http.ResponseWriter, r *http.Request) (notifications.PhoneHome, int, error) {
	receipt := notifications.PhoneHome{
		NodeID: chi.URLParam(r, "id"), RequestID: middleware.GetReqID(r.Context()),
	}
	if len(receipt.RequestID) > 128 || strings.ContainsFunc(receipt.RequestID, unicode.IsControl) {
		receipt.RequestID = ""
	}
	if !validPhoneHomeID(receipt.NodeID) {
		return receipt, http.StatusBadRequest, fmt.Errorf("invalid phone-home id")
	}
	if r.ContentLength == 0 {
		return receipt, 0, nil
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		return receipt, http.StatusUnsupportedMediaType, fmt.Errorf("phone-home requires form data")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPhoneHomeBody)
	if err := r.ParseForm(); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return receipt, http.StatusRequestEntityTooLarge, fmt.Errorf("phone-home body too large")
		}
		return receipt, http.StatusBadRequest, fmt.Errorf("invalid phone-home form")
	}
	for name, target := range map[string]*string{
		"instance_id": &receipt.InstanceID, "hostname": &receipt.Hostname, "fqdn": &receipt.FQDN,
	} {
		values := r.PostForm[name]
		if len(values) > 1 {
			return receipt, http.StatusBadRequest, fmt.Errorf("duplicate phone-home field")
		}
		*target = r.PostForm.Get(name)
		if len(*target) > 256 || strings.ContainsAny(*target, "\r\n\x00") {
			return receipt, http.StatusBadRequest, fmt.Errorf("invalid phone-home field")
		}
	}
	return receipt, 0, nil
}

func validPhoneHomeID(id string) bool {
	if id == "" || len(id) > 256 {
		return false
	}
	for _, ch := range id {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' {
			continue
		}
		if !strings.ContainsRune("._:-", ch) {
			return false
		}
	}
	return true
}
