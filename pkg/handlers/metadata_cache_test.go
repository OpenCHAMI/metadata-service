// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package handlers_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	cloudinitv1 "github.com/openchami/metadata-service/apis/cloud-init.openchami.io/v1"
	"github.com/openchami/metadata-service/pkg/handlers"
	"github.com/openchami/metadata-service/pkg/smdclient"
)

func TestVendorDataHandlerCacheFirstWithBackendChange(t *testing.T) {
	backend := smdclient.NewMockSMDClient()
	backend.AddComponent(&smdclient.Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	backend.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})

	service := smdclient.NewSMDIntegrationService(backend, smdclient.IntegrationOptions{SyncEnabled: false, SyncInterval: time.Hour})
	if _, err := service.IDfromIP("10.0.0.100"); err != nil {
		t.Fatalf("failed to warm ID cache: %v", err)
	}
	if groups, err := service.GroupMembership("x1000c0s0b0n0"); err != nil || len(groups) != 1 || groups[0] != "compute" {
		t.Fatalf("failed to warm group cache, groups=%v err=%v", groups, err)
	}

	backend.AddGroupMembership("x1000c0s0b0n0", []string{"storage"})

	store := &integrationStore{
		clusterDefaults: &handlers.ClusterDefaults{BaseURL: "http://placeholder"},
		instanceInfo:    map[string]*handlers.InstanceInfo{},
		groupData: map[string]*cloudinitv1.Group{
			"compute": {
				Spec: cloudinitv1.GroupSpec{Template: "#cloud-config\ncompute: true\n"},
			},
			"storage": {
				Spec: cloudinitv1.GroupSpec{Template: "#cloud-config\nstorage: true\n"},
			},
		},
	}

	r := chi.NewRouter()
	r.Get("/vendor-data", handlers.VendorDataHandler(service, store))

	srv := httptest.NewServer(r)
	defer srv.Close()
	store.clusterDefaults.BaseURL = srv.URL

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/vendor-data", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("X-Forwarded-For", "10.0.0.100")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, srv.URL+"/compute.yaml") {
		t.Fatalf("expected cached compute group include, got %q", text)
	}
	if strings.Contains(text, srv.URL+"/storage.yaml") {
		t.Fatalf("did not expect storage group include from unsynced backend change, got %q", text)
	}
}

func TestMetaDataHandlerLiveFallbackWhenCacheEmpty(t *testing.T) {
	backend := smdclient.NewMockSMDClient()
	backend.AddComponent(&smdclient.Component{ID: "x1000c0s0b0n0", NID: 1000, Role: "compute", IP: "10.0.0.100"})
	backend.AddGroupMembership("x1000c0s0b0n0", []string{"compute"})

	service := smdclient.NewSMDIntegrationService(backend, smdclient.IntegrationOptions{SyncEnabled: true, SyncInterval: time.Minute})
	store := &integrationStore{
		clusterDefaults: &handlers.ClusterDefaults{
			ClusterName: "testcluster",
			ShortName:   "tc",
			NidLength:   4,
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*cloudinitv1.Group{
			"compute": {Spec: cloudinitv1.GroupSpec{Template: "#cloud-config\ncompute: true\n"}},
		},
	}

	r := chi.NewRouter()
	r.Get("/meta-data", handlers.MetaDataHandler(service, store))

	req := httptest.NewRequest(http.MethodGet, "/meta-data", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.100")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with live fallback, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "instance-id: x1000c0s0b0n0") {
		t.Fatalf("expected instance-id in response, got %s", w.Body.String())
	}
}
