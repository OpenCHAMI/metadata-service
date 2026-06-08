// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package handlers_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cloudinitv1 "github.com/OpenCHAMI/metadata-service/apis/cloud-init.openchami.io/v1"
	"github.com/OpenCHAMI/metadata-service/pkg/handlers"
	"github.com/OpenCHAMI/metadata-service/pkg/smdclient"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

type integrationStore struct {
	clusterDefaults *handlers.ClusterDefaults
	instanceInfo    map[string]*handlers.InstanceInfo
	groupData       map[string]*cloudinitv1.Group
}

func (m *integrationStore) GetClusterDefaults() (*handlers.ClusterDefaults, error) {
	if m.clusterDefaults == nil {
		return nil, fmt.Errorf("no cluster defaults")
	}
	return m.clusterDefaults, nil
}

func (m *integrationStore) GetInstanceInfo(id string) (*handlers.InstanceInfo, error) {
	if info, ok := m.instanceInfo[id]; ok {
		return info, nil
	}
	return nil, fmt.Errorf("no instance info for %s", id)
}

func (m *integrationStore) GetGroupData(name string) (*cloudinitv1.Group, error) {
	if data, ok := m.groupData[name]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("no data for group %s", name)
}

func newIntegrationServer(t *testing.T) (*httptest.Server, *smdclient.MockSMDClient, *integrationStore) {
	t.Helper()

	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:ff",
		IP:   "10.0.0.100",
	})

	store := &integrationStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL:          "http://placeholder",
			CloudProvider:    "OpenCHAMI",
			Region:           "us-test-1",
			AvailabilityZone: "test-az-1",
			ClusterName:      "testcluster",
			ShortName:        "tc",
			NidLength:        4,
			PublicKeys:       []string{"ssh-rsa AAAAB3... default@key"},
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*cloudinitv1.Group{
			"compute": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Compute nodes",
					Template:    "#cloud-config\npackages:\n  - git\n",
					MetaData: map[string]string{
						"custom_key": "custom_value",
					},
				},
			},
			"ntp": {
				Spec: cloudinitv1.GroupSpec{
					Description: "NTP config",
					Template:    "#cloud-config\nntp:\n  servers: [\"ntp.local\"]\n",
				},
			},
			"rack1": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Rack 1 metadata only",
					Template:    "",
					MetaData: map[string]string{
						"syslog_forwarder": "syslog.rack1.local",
					},
				},
			},
		},
	}

	smd.AddGroupMembership("x1000c0s0b0n0", []string{"compute", "ntp", "rack1"})

	r := chi.NewRouter()
	r.Get("/meta-data", handlers.MetaDataHandler(smd, store))
	r.Get("/vendor-data", handlers.VendorDataHandler(smd, store))
	r.Get("/{group}.yaml", handlers.GroupUserDataHandler(smd, store))

	srv := httptest.NewServer(r)
	store.clusterDefaults.BaseURL = srv.URL
	return srv, smd, store
}

func TestIntegrationCloudInitFlow(t *testing.T) {
	srv, _, _ := newIntegrationServer(t)
	defer srv.Close()

	client := &http.Client{}

	metaReq, err := http.NewRequest("GET", srv.URL+"/meta-data", nil)
	if err != nil {
		t.Fatalf("failed to build meta-data request: %v", err)
	}
	metaReq.Header.Set("X-Forwarded-For", "10.0.0.100")

	metaResp, err := client.Do(metaReq)
	if err != nil {
		t.Fatalf("meta-data request failed: %v", err)
	}
	defer func() {
		if err := metaResp.Body.Close(); err != nil {
			t.Errorf("failed to close meta-data response body: %v", err)
		}
	}()

	if metaResp.StatusCode != http.StatusOK {
		t.Fatalf("meta-data status %d", metaResp.StatusCode)
	}

	metaBody, err := io.ReadAll(metaResp.Body)
	if err != nil {
		t.Fatalf("meta-data read failed: %v", err)
	}

	var metadata handlers.MetaData
	if err := yaml.Unmarshal(metaBody, &metadata); err != nil {
		t.Fatalf("meta-data YAML parse failed: %v", err)
	}

	if metadata.InstanceID != "x1000c0s0b0n0" {
		t.Fatalf("unexpected instance-id: %s", metadata.InstanceID)
	}

	if _, ok := metadata.InstanceData.V1.VendorData.Groups["compute"]; !ok {
		t.Fatalf("expected compute group in vendor_data")
	}
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["ntp"]; !ok {
		t.Fatalf("expected ntp group in vendor_data")
	}
	if _, ok := metadata.InstanceData.V1.VendorData.Groups["rack1"]; !ok {
		t.Fatalf("expected rack1 group in vendor_data")
	}

	vendorReq, err := http.NewRequest("GET", srv.URL+"/vendor-data", nil)
	if err != nil {
		t.Fatalf("failed to build vendor-data request: %v", err)
	}
	vendorReq.Header.Set("X-Forwarded-For", "10.0.0.100")

	vendorResp, err := client.Do(vendorReq)
	if err != nil {
		t.Fatalf("vendor-data request failed: %v", err)
	}
	defer func() {
		if err := vendorResp.Body.Close(); err != nil {
			t.Errorf("failed to close vendor-data response body: %v", err)
		}
	}()

	if vendorResp.StatusCode != http.StatusOK {
		t.Fatalf("vendor-data status %d", vendorResp.StatusCode)
	}

	vendorBody, err := io.ReadAll(vendorResp.Body)
	if err != nil {
		t.Fatalf("vendor-data read failed: %v", err)
	}

	vendorText := string(vendorBody)
	if !strings.HasPrefix(vendorText, "#include\n") {
		t.Fatalf("vendor-data does not start with #include")
	}
	if !strings.Contains(vendorText, srv.URL+"/compute.yaml") {
		t.Fatalf("vendor-data missing compute include")
	}
	if !strings.Contains(vendorText, srv.URL+"/ntp.yaml") {
		t.Fatalf("vendor-data missing ntp include")
	}
	if strings.Contains(vendorText, srv.URL+"/rack1.yaml") {
		t.Fatalf("vendor-data should not include rack1 because template is empty")
	}

	groupReq, err := http.NewRequest("GET", srv.URL+"/compute.yaml", nil)
	if err != nil {
		t.Fatalf("failed to build group request: %v", err)
	}
	groupReq.Header.Set("X-Forwarded-For", "10.0.0.100")

	groupResp, err := client.Do(groupReq)
	if err != nil {
		t.Fatalf("group request failed: %v", err)
	}
	defer func() {
		if err := groupResp.Body.Close(); err != nil {
			t.Errorf("failed to close group response body: %v", err)
		}
	}()

	if groupResp.StatusCode != http.StatusOK {
		t.Fatalf("group status %d", groupResp.StatusCode)
	}

	groupBody, err := io.ReadAll(groupResp.Body)
	if err != nil {
		t.Fatalf("group read failed: %v", err)
	}

	if !strings.Contains(string(groupBody), "packages:") {
		t.Fatalf("group response missing expected template content")
	}
}

func TestIntegrationSyslogRackMetadataIsolation(t *testing.T) {
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:ff",
		IP:   "10.0.0.100",
	})
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n1",
		NID:  1001,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:00",
		IP:   "10.0.0.101",
	})

	store := &integrationStore{
		clusterDefaults: &handlers.ClusterDefaults{
			ClusterName: "testcluster",
			ShortName:   "tc",
			NidLength:   4,
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*cloudinitv1.Group{
			"syslog-ng": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Syslog config",
					Template:    "#cloud-config\nsyslog:\n  forwarder: {% for name, group in vendor_data.groups %}{% if group.syslog_forwarder %}{{ group.syslog_forwarder }}{% endif %}{% endfor %}\n",
					MetaData:    map[string]string{},
				},
			},
			"rack1": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Rack 1 metadata",
					Template:    "",
					MetaData: map[string]string{
						"syslog_forwarder": "syslog.rack1.local",
					},
				},
			},
			"rack2": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Rack 2 metadata",
					Template:    "",
					MetaData: map[string]string{
						"syslog_forwarder": "syslog.rack2.local",
					},
				},
			},
		},
	}

	smd.AddGroupMembership("x1000c0s0b0n0", []string{"syslog-ng", "rack1"})
	smd.AddGroupMembership("x1000c0s0b0n1", []string{"syslog-ng", "rack2"})

	r := chi.NewRouter()
	r.Get("/{group}.yaml", handlers.GroupUserDataHandler(smd, store))

	srv := httptest.NewServer(r)
	defer srv.Close()

	client := &http.Client{}

	for ip, forwarder := range map[string]string{
		"10.0.0.100": "syslog.rack1.local",
		"10.0.0.101": "syslog.rack2.local",
	} {
		req, err := http.NewRequest("GET", srv.URL+"/syslog-ng.yaml", nil)
		if err != nil {
			t.Fatalf("failed to build syslog request: %v", err)
		}
		req.Header.Set("X-Forwarded-For", ip)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("syslog request failed: %v", err)
		}
		body, err := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("failed to close syslog response body: %v", closeErr)
		}
		if err != nil {
			t.Fatalf("syslog response read failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("syslog response status %d", resp.StatusCode)
		}

		if !strings.Contains(string(body), forwarder) {
			t.Fatalf("syslog response missing rack metadata %s", forwarder)
		}
	}
}

func TestIntegrationPerHostArtifactsOverWireGuardIdentity(t *testing.T) {
	smd := smdclient.NewMockSMDClient()
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n0",
		NID:  1000,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:ff",
		IP:   "10.0.0.100",
	})
	smd.AddComponent(&smdclient.Component{
		ID:   "x1000c0s0b0n1",
		NID:  1001,
		Role: "compute",
		MAC:  "aa:bb:cc:dd:ee:00",
		IP:   "10.0.0.101",
	})
	if err := smd.AddWGIP("x1000c0s0b0n0", "10.100.1.25"); err != nil {
		t.Fatalf("AddWGIP returned error: %v", err)
	}
	if err := smd.AddWGIP("x1000c0s0b0n1", "10.100.1.26"); err != nil {
		t.Fatalf("AddWGIP returned error: %v", err)
	}
	smd.AddGroupMembership("x1000c0s0b0n0", []string{"syslog-ng", "rack1"})
	smd.AddGroupMembership("x1000c0s0b0n1", []string{"syslog-ng", "rack2"})

	store := &integrationStore{
		clusterDefaults: &handlers.ClusterDefaults{
			BaseURL:       "http://metadata.test",
			ClusterName:   "testcluster",
			ShortName:     "tc",
			NidLength:     4,
			CloudProvider: "OpenCHAMI",
			Region:        "us-test-1",
		},
		instanceInfo: map[string]*handlers.InstanceInfo{},
		groupData: map[string]*cloudinitv1.Group{
			"syslog-ng": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Syslog config",
					Template:    "#cloud-config\nsyslog:\n  forwarder: {% for name, group in vendor_data.groups %}{% if group.syslog_forwarder %}{{ group.syslog_forwarder }}{% endif %}{% endfor %}\n",
				},
			},
			"rack1": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Rack 1 metadata",
					Template:    "",
					MetaData: map[string]string{
						"syslog_forwarder": "syslog.rack1.local",
					},
				},
			},
			"rack2": {
				Spec: cloudinitv1.GroupSpec{
					Description: "Rack 2 metadata",
					Template:    "",
					MetaData: map[string]string{
						"syslog_forwarder": "syslog.rack2.local",
					},
				},
			},
		},
	}

	r := chi.NewRouter()
	r.Get("/meta-data", handlers.MetaDataHandler(smd, store))
	r.Get("/vendor-data", handlers.VendorDataHandler(smd, store))
	r.Get("/{group}.yaml", handlers.GroupUserDataHandler(smd, store))

	testCases := []struct {
		name                string
		wgIP                string
		instanceID          string
		expectedRack        string
		expectedForwarder   string
		unexpectedRack      string
		unexpectedForwarder string
	}{
		{
			name:                "node0",
			wgIP:                "10.100.1.25",
			instanceID:          "x1000c0s0b0n0",
			expectedRack:        "rack1",
			expectedForwarder:   "syslog.rack1.local",
			unexpectedRack:      "rack2",
			unexpectedForwarder: "syslog.rack2.local",
		},
		{
			name:                "node1",
			wgIP:                "10.100.1.26",
			instanceID:          "x1000c0s0b0n1",
			expectedRack:        "rack2",
			expectedForwarder:   "syslog.rack2.local",
			unexpectedRack:      "rack1",
			unexpectedForwarder: "syslog.rack1.local",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metaReq := httptest.NewRequest(http.MethodGet, "/meta-data", nil)
			metaReq.RemoteAddr = tc.wgIP + ":12345"
			metaResp := httptest.NewRecorder()
			r.ServeHTTP(metaResp, metaReq)

			if metaResp.Code != http.StatusOK {
				t.Fatalf("meta-data status %d: %s", metaResp.Code, metaResp.Body.String())
			}

			var metadata handlers.MetaData
			if err := yaml.Unmarshal(metaResp.Body.Bytes(), &metadata); err != nil {
				t.Fatalf("meta-data YAML parse failed: %v", err)
			}
			if metadata.InstanceID != tc.instanceID {
				t.Fatalf("expected instance-id %s, got %s", tc.instanceID, metadata.InstanceID)
			}
			if _, ok := metadata.InstanceData.V1.VendorData.Groups[tc.expectedRack]; !ok {
				t.Fatalf("expected %s in vendor_data.groups", tc.expectedRack)
			}
			if _, ok := metadata.InstanceData.V1.VendorData.Groups[tc.unexpectedRack]; ok {
				t.Fatalf("did not expect %s in vendor_data.groups", tc.unexpectedRack)
			}

			vendorReq := httptest.NewRequest(http.MethodGet, "/vendor-data", nil)
			vendorReq.RemoteAddr = tc.wgIP + ":12345"
			vendorResp := httptest.NewRecorder()
			r.ServeHTTP(vendorResp, vendorReq)

			if vendorResp.Code != http.StatusOK {
				t.Fatalf("vendor-data status %d: %s", vendorResp.Code, vendorResp.Body.String())
			}

			vendorText := vendorResp.Body.String()
			if !strings.Contains(vendorText, "http://metadata.test/syslog-ng.yaml") {
				t.Fatalf("expected syslog-ng include, got %q", vendorText)
			}
			if strings.Contains(vendorText, tc.expectedRack+".yaml") {
				t.Fatalf("did not expect metadata-only group %s in include list: %q", tc.expectedRack, vendorText)
			}

			groupReq := httptest.NewRequest(http.MethodGet, "/syslog-ng.yaml", nil)
			groupReq.RemoteAddr = tc.wgIP + ":12345"
			groupResp := httptest.NewRecorder()
			r.ServeHTTP(groupResp, groupReq)

			if groupResp.Code != http.StatusOK {
				t.Fatalf("group status %d: %s", groupResp.Code, groupResp.Body.String())
			}

			rendered := groupResp.Body.String()
			if !strings.Contains(rendered, tc.expectedForwarder) {
				t.Fatalf("expected rendered output to contain %s, got %q", tc.expectedForwarder, rendered)
			}
			if strings.Contains(rendered, tc.unexpectedForwarder) {
				t.Fatalf("did not expect rendered output to contain %s, got %q", tc.unexpectedForwarder, rendered)
			}
		})
	}
}
