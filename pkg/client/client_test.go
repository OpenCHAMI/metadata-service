// SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openchami/fabrica/pkg/fabrica"
	v1 "github.com/openchami/metadata-service/apis/cloud-init.openchami.io/v1"
	"github.com/openchami/metadata-service/pkg/client"
	"github.com/stretchr/testify/require"
)

// Test fixtures - minimal examples for testing
var (
	// ClusterDefaults fixtures
	clusterDefaultsJSON = `{
		"metadata": {"uid": "cd-uid-1", "name": "test-cluster"},
		"spec": {
			"base_url": "http://test.local",
			"cloud_provider": "OpenCHAMI",
			"cluster_name": "testcluster"
		},
		"status": {}
	}`

	clusterDefaultsListJSON = `[` + clusterDefaultsJSON + `]`

	// Group fixtures (with template validation scenarios)
	groupValidJSON = `{
		"metadata": {"uid": "group-uid-1", "name": "compute"},
		"spec": {
			"template": "#cloud-config\nhostname: {{hostname}}",
			"metaData": {"hostname": "test-host"}
		},
		"status": {"valid": true}
	}`

	groupListJSON = `[` + groupValidJSON + `]`

	// InstanceInfo fixtures
	instanceInfoJSON = `{
		"metadata": {"uid": "ii-uid-1", "name": "node1"},
		"spec": {
			"instance_id": "node1",
			"hostname": "node1.cluster.local"
		},
		"status": {}
	}`

	instanceInfoListJSON = `[` + instanceInfoJSON + `]`

	// WireGuardPeer fixtures
	wireguardPeerJSON = `{
		"metadata": {"uid": "wg-uid-1", "name": "peer1"},
		"spec": {
			"public_key": "test-public-key",
			"allowed_ip": "10.100.0.1/32"
		},
		"status": {}
	}`

	wireguardPeerListJSON = `[` + wireguardPeerJSON + `]`

	// Error response fixtures
	errorResponseJSON      = `{"error": "resource not found"}`
	validationErrorJSON    = `{"error": "validation failed: missing required field"}`
	serverErrorJSON        = `{"error": "internal server error"}`
	unauthorizedErrorJSON  = `{"error": "unauthorized"}`
	forbiddenErrorJSON     = `{"error": "forbidden"}`
	conflictErrorJSON      = `{"error": "resource already exists"}`
	serviceUnavailableJSON = `{"error": "service unavailable"}`

	// Health response fixture
	healthResponseJSON = `{"status": "ok", "version": "1.0.0"}`
)

// Helper functions

// setupTestServer creates a client connected to a mock HTTP server
func setupTestServer(t *testing.T, handler http.HandlerFunc) (*client.Client, *httptest.Server) {
	server := httptest.NewServer(handler)
	c, err := client.NewClient(server.URL, nil, client.DefaultLogger())
	require.NoError(t, err)
	return c, server
}

// verifyRequestBasics checks common request properties
func verifyRequestBasics(t *testing.T, r *http.Request, method, path string) {
	require.Equal(t, method, r.Method)
	require.Equal(t, path, r.URL.Path)
}

// verifyAuthHeader checks Authorization header
func verifyAuthHeader(t *testing.T, r *http.Request, expectedToken string) {
	if expectedToken != "" {
		require.Equal(t, "Bearer "+expectedToken, r.Header.Get("Authorization"))
	} else {
		require.Empty(t, r.Header.Get("Authorization"))
	}
}

// verifyContentType checks Content-Type header
func verifyContentType(t *testing.T, r *http.Request, expectedType string) {
	if expectedType != "" {
		require.Contains(t, r.Header.Get("Content-Type"), expectedType)
	}
}

// verifyAcceptHeader checks Accept header
func verifyAcceptHeader(t *testing.T, r *http.Request, expectedType string) {
	if expectedType != "" {
		require.Contains(t, r.Header.Get("Accept"), expectedType)
	}
}

// respondJSON writes a JSON response with given status code
func respondJSON(w http.ResponseWriter, statusCode int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}

// readRequestBody reads and returns the request body as a string
func readRequestBody(t *testing.T, r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return string(body)
}

// Client Initialization Tests

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		httpClient  *http.Client
		expectError bool
	}{
		{
			name:        "valid URL creates client successfully",
			baseURL:     "http://localhost:8080",
			httpClient:  nil,
			expectError: false,
		},
		{
			name:        "valid HTTPS URL",
			baseURL:     "https://api.example.com",
			httpClient:  nil,
			expectError: false,
		},
		{
			name:        "URL with path",
			baseURL:     "http://localhost:8080/api/v1",
			httpClient:  nil,
			expectError: false,
		},
		{
			name:        "custom http.Client is used",
			baseURL:     "http://localhost:8080",
			httpClient:  &http.Client{Timeout: 30 * time.Second},
			expectError: false,
		},
		{
			name:        "invalid URL returns error",
			baseURL:     "://invalid-url",
			httpClient:  nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := client.NewClient(tt.baseURL, tt.httpClient, client.DefaultLogger())
			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, c)
			} else {
				require.NoError(t, err)
				require.NotNil(t, c)
			}
		})
	}
}

func TestNewClientWithBearerToken(t *testing.T) {
	const testToken = "test-bearer-token-12345"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify token is present in request
		verifyAuthHeader(t, r, testToken)
		respondJSON(w, http.StatusOK, healthResponseJSON)
	}))
	defer server.Close()

	c, err := client.NewClientWithBearerToken(server.URL, testToken, nil, client.DefaultLogger())
	require.NoError(t, err)
	require.NotNil(t, c)

	// Make a request to verify token is sent
	_, err = c.GetHealth(context.Background())
	require.NoError(t, err)
}

func TestClientWithVersion(t *testing.T) {
	const testVersion = "v1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify version in Accept and Content-Type headers
		acceptHeader := r.Header.Get("Accept")
		require.Contains(t, acceptHeader, "version="+testVersion)
		respondJSON(w, http.StatusOK, healthResponseJSON)
	}))
	defer server.Close()

	originalClient, err := client.NewClient(server.URL, nil, client.DefaultLogger())
	require.NoError(t, err)

	// Create versioned client
	versionedClient := originalClient.WithVersion(testVersion)
	require.NotNil(t, versionedClient)

	// Verify versioned client sends version header
	_, err = versionedClient.GetHealth(context.Background())
	require.NoError(t, err)
}

func TestClientWithBearerToken(t *testing.T) {
	const testToken = "test-token-67890"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyAuthHeader(t, r, testToken)
		respondJSON(w, http.StatusOK, healthResponseJSON)
	}))
	defer server.Close()

	originalClient, err := client.NewClient(server.URL, nil, client.DefaultLogger())
	require.NoError(t, err)

	// Create client with token
	tokenClient := originalClient.WithBearerToken(testToken)
	require.NotNil(t, tokenClient)

	// Verify token is sent
	_, err = tokenClient.GetHealth(context.Background())
	require.NoError(t, err)
}

// Health Endpoint Tests

func TestGetHealth(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		responseBody string
		expectError  bool
	}{
		{
			name:         "success returns health response",
			statusCode:   http.StatusOK,
			responseBody: healthResponseJSON,
			expectError:  false,
		},
		{
			name:         "server error returns error",
			statusCode:   http.StatusInternalServerError,
			responseBody: serverErrorJSON,
			expectError:  true,
		},
		{
			name:         "service unavailable returns error",
			statusCode:   http.StatusServiceUnavailable,
			responseBody: serviceUnavailableJSON,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "GET", "/health")
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := c.GetHealth(context.Background())
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

// Resource List Tests (Table-Driven)

func TestGetResourceList(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context) (interface{}, error)
		validateFunc func(*testing.T, interface{})
	}{
		{
			name:         "GetClusterDefaultss success with items",
			path:         "/clusterdefaultss",
			responseBody: clusterDefaultsListJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetClusterDefaultss(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.ClusterDefaults)
				require.Len(t, list, 1)
				require.Equal(t, "cd-uid-1", list[0].Metadata.UID)
			},
		},
		{
			name:         "GetClusterDefaultss empty list",
			path:         "/clusterdefaultss",
			responseBody: `[]`,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetClusterDefaultss(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.ClusterDefaults)
				require.Len(t, list, 0)
			},
		},
		{
			name:         "GetGroups success with items",
			path:         "/groups",
			responseBody: groupListJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetGroups(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.Group)
				require.Len(t, list, 1)
				require.Equal(t, "group-uid-1", list[0].Metadata.UID)
			},
		},
		{
			name:         "GetGroups empty list",
			path:         "/groups",
			responseBody: `[]`,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetGroups(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.Group)
				require.Len(t, list, 0)
			},
		},
		{
			name:         "GetInstanceInfos success with items",
			path:         "/instanceinfos",
			responseBody: instanceInfoListJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetInstanceInfos(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.InstanceInfo)
				require.Len(t, list, 1)
				require.Equal(t, "ii-uid-1", list[0].Metadata.UID)
			},
		},
		{
			name:         "GetWireGuardPeers success with items",
			path:         "/wireguardpeers",
			responseBody: wireguardPeerListJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetWireGuardPeers(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.WireGuardPeer)
				require.Len(t, list, 1)
				require.Equal(t, "wg-uid-1", list[0].Metadata.UID)
			},
		},
		{
			name:         "GetClusterDefaultss server error",
			path:         "/clusterdefaultss",
			responseBody: serverErrorJSON,
			statusCode:   http.StatusInternalServerError,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetClusterDefaultss(ctx)
			},
			validateFunc: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "GET", tt.path)
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background())
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
			}
		})
	}
}

// Resource Get By UID Tests (Table-Driven)

func TestGetResourceByUID(t *testing.T) {
	tests := []struct {
		name         string
		uid          string
		path         string
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context, string) (interface{}, error)
		validateFunc func(*testing.T, interface{})
	}{
		{
			name:         "GetClusterDefaults success",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1",
			responseBody: clusterDefaultsJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string) (interface{}, error) {
				return c.GetClusterDefaults(ctx, uid)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.ClusterDefaults)
				require.Equal(t, "cd-uid-1", resource.Metadata.UID)
			},
		},
		{
			name:         "GetClusterDefaults not found",
			uid:          "missing-uid",
			path:         "/clusterdefaultss/missing-uid",
			responseBody: errorResponseJSON,
			statusCode:   http.StatusNotFound,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string) (interface{}, error) {
				return c.GetClusterDefaults(ctx, uid)
			},
			validateFunc: nil,
		},
		{
			name:         "GetGroup success",
			uid:          "group-uid-1",
			path:         "/groups/group-uid-1",
			responseBody: groupValidJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string) (interface{}, error) {
				return c.GetGroup(ctx, uid)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.Group)
				require.Equal(t, "group-uid-1", resource.Metadata.UID)
				require.True(t, resource.Status.Valid)
			},
		},
		{
			name:         "GetGroup not found",
			uid:          "missing-uid",
			path:         "/groups/missing-uid",
			responseBody: errorResponseJSON,
			statusCode:   http.StatusNotFound,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string) (interface{}, error) {
				return c.GetGroup(ctx, uid)
			},
			validateFunc: nil,
		},
		{
			name:         "GetInstanceInfo success",
			uid:          "ii-uid-1",
			path:         "/instanceinfos/ii-uid-1",
			responseBody: instanceInfoJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string) (interface{}, error) {
				return c.GetInstanceInfo(ctx, uid)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.InstanceInfo)
				require.Equal(t, "ii-uid-1", resource.Metadata.UID)
			},
		},
		{
			name:         "GetWireGuardPeer success",
			uid:          "wg-uid-1",
			path:         "/wireguardpeers/wg-uid-1",
			responseBody: wireguardPeerJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string) (interface{}, error) {
				return c.GetWireGuardPeer(ctx, uid)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.WireGuardPeer)
				require.Equal(t, "wg-uid-1", resource.Metadata.UID)
			},
		},
		{
			name:         "GetClusterDefaults server error",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1",
			responseBody: serverErrorJSON,
			statusCode:   http.StatusInternalServerError,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string) (interface{}, error) {
				return c.GetClusterDefaults(ctx, uid)
			},
			validateFunc: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "GET", tt.path)
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background(), tt.uid)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
			}
		})
	}
}

// Resource Create Tests (Table-Driven)

func TestCreateResource(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		requestBody  interface{}
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context, interface{}) (interface{}, error)
		validateFunc func(*testing.T, interface{})
	}{
		{
			name: "CreateClusterDefaults success",
			path: "/clusterdefaultss",
			requestBody: client.CreateClusterDefaultsRequest{
				Metadata: fabrica.Metadata{Name: "test-cluster"},
				Spec: v1.ClusterDefaultsSpec{
					BaseURL:       "http://test.local",
					CloudProvider: "OpenCHAMI",
					ClusterName:   "testcluster",
				},
			},
			responseBody: clusterDefaultsJSON,
			statusCode:   http.StatusCreated,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, req interface{}) (interface{}, error) {
				return c.CreateClusterDefaults(ctx, req.(client.CreateClusterDefaultsRequest))
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.ClusterDefaults)
				require.Equal(t, "cd-uid-1", resource.Metadata.UID)
			},
		},
		{
			name: "CreateClusterDefaults validation error",
			path: "/clusterdefaultss",
			requestBody: client.CreateClusterDefaultsRequest{
				Metadata: fabrica.Metadata{Name: ""},
				Spec:     v1.ClusterDefaultsSpec{},
			},
			responseBody: validationErrorJSON,
			statusCode:   http.StatusBadRequest,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, req interface{}) (interface{}, error) {
				return c.CreateClusterDefaults(ctx, req.(client.CreateClusterDefaultsRequest))
			},
			validateFunc: nil,
		},
		{
			name: "CreateGroup success with valid template",
			path: "/groups",
			requestBody: client.CreateGroupRequest{
				Metadata: fabrica.Metadata{Name: "compute"},
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{hostname}}",
					MetaData: map[string]string{"hostname": "test-host"},
				},
			},
			responseBody: groupValidJSON,
			statusCode:   http.StatusCreated,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, req interface{}) (interface{}, error) {
				return c.CreateGroup(ctx, req.(client.CreateGroupRequest))
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.Group)
				require.Equal(t, "group-uid-1", resource.Metadata.UID)
				require.True(t, resource.Status.Valid)
			},
		},
		{
			name: "CreateGroup validation error with invalid template",
			path: "/groups",
			requestBody: client.CreateGroupRequest{
				Metadata: fabrica.Metadata{Name: "invalid"},
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{missing_var}}",
					MetaData: map[string]string{"hostname": "test-host"},
				},
			},
			responseBody: validationErrorJSON,
			statusCode:   http.StatusBadRequest,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, req interface{}) (interface{}, error) {
				return c.CreateGroup(ctx, req.(client.CreateGroupRequest))
			},
			validateFunc: nil,
		},
		{
			name: "CreateInstanceInfo success",
			path: "/instanceinfos",
			requestBody: client.CreateInstanceInfoRequest{
				Metadata: fabrica.Metadata{Name: "node1"},
				Spec: v1.InstanceInfoSpec{
					InstanceID: "node1",
					Hostname:   "node1.cluster.local",
				},
			},
			responseBody: instanceInfoJSON,
			statusCode:   http.StatusCreated,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, req interface{}) (interface{}, error) {
				return c.CreateInstanceInfo(ctx, req.(client.CreateInstanceInfoRequest))
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.InstanceInfo)
				require.Equal(t, "ii-uid-1", resource.Metadata.UID)
			},
		},
		{
			name: "CreateWireGuardPeer success",
			path: "/wireguardpeers",
			requestBody: client.CreateWireGuardPeerRequest{
				Metadata: fabrica.Metadata{Name: "peer1"},
				Spec: v1.WireGuardPeerSpec{
					PublicKey: "test-public-key",
					AllowedIP: "10.100.0.1/32",
				},
			},
			responseBody: wireguardPeerJSON,
			statusCode:   http.StatusCreated,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, req interface{}) (interface{}, error) {
				return c.CreateWireGuardPeer(ctx, req.(client.CreateWireGuardPeerRequest))
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.WireGuardPeer)
				require.Equal(t, "wg-uid-1", resource.Metadata.UID)
			},
		},
		{
			name: "CreateClusterDefaults conflict error",
			path: "/clusterdefaultss",
			requestBody: client.CreateClusterDefaultsRequest{
				Metadata: fabrica.Metadata{Name: "existing"},
				Spec:     v1.ClusterDefaultsSpec{},
			},
			responseBody: conflictErrorJSON,
			statusCode:   http.StatusConflict,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, req interface{}) (interface{}, error) {
				return c.CreateClusterDefaults(ctx, req.(client.CreateClusterDefaultsRequest))
			},
			validateFunc: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "POST", tt.path)
				verifyContentType(t, r, "application/json")

				// Verify request body is valid JSON
				body := readRequestBody(t, r)
				require.NotEmpty(t, body)
				var jsonCheck map[string]interface{}
				err := json.Unmarshal([]byte(body), &jsonCheck)
				require.NoError(t, err, "request body should be valid JSON")

				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background(), tt.requestBody)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
			}
		})
	}
}

// Resource Update Tests (Table-Driven)

func TestUpdateResource(t *testing.T) {
	tests := []struct {
		name         string
		uid          string
		path         string
		requestBody  interface{}
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context, string, interface{}) (interface{}, error)
		validateFunc func(*testing.T, interface{})
	}{
		{
			name: "UpdateClusterDefaults success",
			uid:  "cd-uid-1",
			path: "/clusterdefaultss/cd-uid-1",
			requestBody: client.UpdateClusterDefaultsRequest{
				Spec: v1.ClusterDefaultsSpec{
					BaseURL: "http://updated.local",
				},
			},
			responseBody: clusterDefaultsJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, req interface{}) (interface{}, error) {
				return c.UpdateClusterDefaults(ctx, uid, req.(client.UpdateClusterDefaultsRequest))
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.ClusterDefaults)
				require.Equal(t, "cd-uid-1", resource.Metadata.UID)
			},
		},
		{
			name: "UpdateClusterDefaults not found",
			uid:  "missing-uid",
			path: "/clusterdefaultss/missing-uid",
			requestBody: client.UpdateClusterDefaultsRequest{
				Spec: v1.ClusterDefaultsSpec{},
			},
			responseBody: errorResponseJSON,
			statusCode:   http.StatusNotFound,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string, req interface{}) (interface{}, error) {
				return c.UpdateClusterDefaults(ctx, uid, req.(client.UpdateClusterDefaultsRequest))
			},
			validateFunc: nil,
		},
		{
			name: "UpdateGroup success",
			uid:  "group-uid-1",
			path: "/groups/group-uid-1",
			requestBody: client.UpdateGroupRequest{
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{hostname}}",
					MetaData: map[string]string{"hostname": "updated-host"},
				},
			},
			responseBody: groupValidJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, req interface{}) (interface{}, error) {
				return c.UpdateGroup(ctx, uid, req.(client.UpdateGroupRequest))
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.Group)
				require.Equal(t, "group-uid-1", resource.Metadata.UID)
			},
		},
		{
			name: "UpdateGroup template validation error",
			uid:  "group-uid-1",
			path: "/groups/group-uid-1",
			requestBody: client.UpdateGroupRequest{
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{missing_var}}",
					MetaData: map[string]string{"hostname": "test-host"},
				},
			},
			responseBody: validationErrorJSON,
			statusCode:   http.StatusBadRequest,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string, req interface{}) (interface{}, error) {
				return c.UpdateGroup(ctx, uid, req.(client.UpdateGroupRequest))
			},
			validateFunc: nil,
		},
		{
			name: "UpdateInstanceInfo success",
			uid:  "ii-uid-1",
			path: "/instanceinfos/ii-uid-1",
			requestBody: client.UpdateInstanceInfoRequest{
				Spec: v1.InstanceInfoSpec{
					Hostname: "updated-node.cluster.local",
				},
			},
			responseBody: instanceInfoJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, req interface{}) (interface{}, error) {
				return c.UpdateInstanceInfo(ctx, uid, req.(client.UpdateInstanceInfoRequest))
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.InstanceInfo)
				require.Equal(t, "ii-uid-1", resource.Metadata.UID)
			},
		},
		{
			name: "UpdateWireGuardPeer success",
			uid:  "wg-uid-1",
			path: "/wireguardpeers/wg-uid-1",
			requestBody: client.UpdateWireGuardPeerRequest{
				Spec: v1.WireGuardPeerSpec{
					AllowedIP: "10.100.0.2/32",
				},
			},
			responseBody: wireguardPeerJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, req interface{}) (interface{}, error) {
				return c.UpdateWireGuardPeer(ctx, uid, req.(client.UpdateWireGuardPeerRequest))
			},
			validateFunc: func(t *testing.T, result interface{}) {
				resource := result.(*v1.WireGuardPeer)
				require.Equal(t, "wg-uid-1", resource.Metadata.UID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "PUT", tt.path)
				verifyContentType(t, r, "application/json")
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background(), tt.uid, tt.requestBody)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
			}
		})
	}
}

// Resource Patch Tests (Table-Driven)

func TestPatchResource(t *testing.T) {
	tests := []struct {
		name         string
		uid          string
		path         string
		patchData    []byte
		contentType  string
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context, string, []byte, string) (interface{}, error)
	}{
		{
			name:         "PatchClusterDefaults with JSON patch",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1",
			patchData:    []byte(`[{"op":"replace","path":"/spec/base_url","value":"http://patched.local"}]`),
			contentType:  "application/json-patch+json",
			responseBody: clusterDefaultsJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchClusterDefaults(ctx, uid, data, ct)
			},
		},
		{
			name:         "PatchClusterDefaults with merge patch",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1",
			patchData:    []byte(`{"spec":{"base_url":"http://merged.local"}}`),
			contentType:  "application/merge-patch+json",
			responseBody: clusterDefaultsJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchClusterDefaults(ctx, uid, data, ct)
			},
		},
		{
			name:         "PatchGroup with JSON patch",
			uid:          "group-uid-1",
			path:         "/groups/group-uid-1",
			patchData:    []byte(`[{"op":"replace","path":"/spec/template","value":"#cloud-config\nupdated"}]`),
			contentType:  "application/json-patch+json",
			responseBody: groupValidJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchGroup(ctx, uid, data, ct)
			},
		},
		{
			name:         "PatchInstanceInfo with JSON patch",
			uid:          "ii-uid-1",
			path:         "/instanceinfos/ii-uid-1",
			patchData:    []byte(`[{"op":"replace","path":"/spec/hostname","value":"patched.local"}]`),
			contentType:  "application/json-patch+json",
			responseBody: instanceInfoJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchInstanceInfo(ctx, uid, data, ct)
			},
		},
		{
			name:         "PatchWireGuardPeer with JSON patch",
			uid:          "wg-uid-1",
			path:         "/wireguardpeers/wg-uid-1",
			patchData:    []byte(`[{"op":"add","path":"/spec/allowed_ips/-","value":"10.100.0.3/32"}]`),
			contentType:  "application/json-patch+json",
			responseBody: wireguardPeerJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchWireGuardPeer(ctx, uid, data, ct)
			},
		},
		{
			name:         "PatchClusterDefaults not found",
			uid:          "missing-uid",
			path:         "/clusterdefaultss/missing-uid",
			patchData:    []byte(`[{"op":"replace","path":"/spec/base_url","value":"http://test.local"}]`),
			contentType:  "application/json-patch+json",
			responseBody: errorResponseJSON,
			statusCode:   http.StatusNotFound,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchClusterDefaults(ctx, uid, data, ct)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "PATCH", tt.path)
				verifyContentType(t, r, tt.contentType)
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background(), tt.uid, tt.patchData, tt.contentType)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

// Resource Status Update Tests (Table-Driven)

func TestUpdateResourceStatus(t *testing.T) {
	tests := []struct {
		name         string
		uid          string
		path         string
		status       interface{}
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context, string, interface{}) (interface{}, error)
	}{
		{
			name:         "UpdateClusterDefaultsStatus success",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1/status",
			status:       v1.ClusterDefaultsStatus{},
			responseBody: clusterDefaultsJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, status interface{}) (interface{}, error) {
				return c.UpdateClusterDefaultsStatus(ctx, uid, status.(v1.ClusterDefaultsStatus))
			},
		},
		{
			name:         "UpdateGroupStatus success",
			uid:          "group-uid-1",
			path:         "/groups/group-uid-1/status",
			status:       v1.GroupStatus{Valid: true},
			responseBody: groupValidJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, status interface{}) (interface{}, error) {
				return c.UpdateGroupStatus(ctx, uid, status.(v1.GroupStatus))
			},
		},
		{
			name:         "UpdateInstanceInfoStatus success",
			uid:          "ii-uid-1",
			path:         "/instanceinfos/ii-uid-1/status",
			status:       v1.InstanceInfoStatus{},
			responseBody: instanceInfoJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, status interface{}) (interface{}, error) {
				return c.UpdateInstanceInfoStatus(ctx, uid, status.(v1.InstanceInfoStatus))
			},
		},
		{
			name:         "UpdateWireGuardPeerStatus success",
			uid:          "wg-uid-1",
			path:         "/wireguardpeers/wg-uid-1/status",
			status:       v1.WireGuardPeerStatus{},
			responseBody: wireguardPeerJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, status interface{}) (interface{}, error) {
				return c.UpdateWireGuardPeerStatus(ctx, uid, status.(v1.WireGuardPeerStatus))
			},
		},
		{
			name:         "UpdateClusterDefaultsStatus not found",
			uid:          "missing-uid",
			path:         "/clusterdefaultss/missing-uid/status",
			status:       v1.ClusterDefaultsStatus{},
			responseBody: errorResponseJSON,
			statusCode:   http.StatusNotFound,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string, status interface{}) (interface{}, error) {
				return c.UpdateClusterDefaultsStatus(ctx, uid, status.(v1.ClusterDefaultsStatus))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "PUT", tt.path)
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background(), tt.uid, tt.status)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

// Resource Status Patch Tests (Table-Driven)

func TestPatchResourceStatus(t *testing.T) {
	tests := []struct {
		name         string
		uid          string
		path         string
		patchData    []byte
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context, string, []byte) (interface{}, error)
	}{
		{
			name:         "PatchClusterDefaultsStatus success",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1/status",
			patchData:    []byte(`[{"op":"replace","path":"/ready","value":true}]`),
			responseBody: clusterDefaultsJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte) (interface{}, error) {
				return c.PatchClusterDefaultsStatus(ctx, uid, data)
			},
		},
		{
			name:         "PatchGroupStatus success",
			uid:          "group-uid-1",
			path:         "/groups/group-uid-1/status",
			patchData:    []byte(`[{"op":"replace","path":"/valid","value":false}]`),
			responseBody: groupValidJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte) (interface{}, error) {
				return c.PatchGroupStatus(ctx, uid, data)
			},
		},
		{
			name:         "PatchInstanceInfoStatus success",
			uid:          "ii-uid-1",
			path:         "/instanceinfos/ii-uid-1/status",
			patchData:    []byte(`[{"op":"add","path":"/ready","value":true}]`),
			responseBody: instanceInfoJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte) (interface{}, error) {
				return c.PatchInstanceInfoStatus(ctx, uid, data)
			},
		},
		{
			name:         "PatchWireGuardPeerStatus success",
			uid:          "wg-uid-1",
			path:         "/wireguardpeers/wg-uid-1/status",
			patchData:    []byte(`[{"op":"add","path":"/connected","value":true}]`),
			responseBody: wireguardPeerJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte) (interface{}, error) {
				return c.PatchWireGuardPeerStatus(ctx, uid, data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "PATCH", tt.path)
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background(), tt.uid, tt.patchData)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

func TestPatchResourceStatusWithType(t *testing.T) {
	tests := []struct {
		name         string
		uid          string
		path         string
		patchData    []byte
		contentType  string
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context, string, []byte, string) (interface{}, error)
	}{
		{
			name:         "PatchClusterDefaultsStatusWithType JSON patch",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1/status",
			patchData:    []byte(`[{"op":"replace","path":"/ready","value":true}]`),
			contentType:  "application/json-patch+json",
			responseBody: clusterDefaultsJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchClusterDefaultsStatusWithType(ctx, uid, data, ct)
			},
		},
		{
			name:         "PatchGroupStatusWithType merge patch",
			uid:          "group-uid-1",
			path:         "/groups/group-uid-1/status",
			patchData:    []byte(`{"valid":false}`),
			contentType:  "application/merge-patch+json",
			responseBody: groupValidJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchGroupStatusWithType(ctx, uid, data, ct)
			},
		},
		{
			name:         "PatchInstanceInfoStatusWithType JSON patch",
			uid:          "ii-uid-1",
			path:         "/instanceinfos/ii-uid-1/status",
			patchData:    []byte(`[{"op":"add","path":"/ready","value":true}]`),
			contentType:  "application/json-patch+json",
			responseBody: instanceInfoJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchInstanceInfoStatusWithType(ctx, uid, data, ct)
			},
		},
		{
			name:         "PatchWireGuardPeerStatusWithType JSON patch",
			uid:          "wg-uid-1",
			path:         "/wireguardpeers/wg-uid-1/status",
			patchData:    []byte(`[{"op":"add","path":"/connected","value":true}]`),
			contentType:  "application/json-patch+json",
			responseBody: wireguardPeerJSON,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string, data []byte, ct string) (interface{}, error) {
				return c.PatchWireGuardPeerStatusWithType(ctx, uid, data, ct)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "PATCH", tt.path)
				verifyContentType(t, r, tt.contentType)
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background(), tt.uid, tt.patchData, tt.contentType)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

// Resource Delete Tests (Table-Driven)

func TestDeleteResource(t *testing.T) {
	tests := []struct {
		name         string
		uid          string
		path         string
		responseBody string
		statusCode   int
		expectError  bool
		testFunc     func(*client.Client, context.Context, string) error
	}{
		{
			name:         "DeleteClusterDefaults success",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1",
			responseBody: `{"message":"deleted","uid":"cd-uid-1"}`,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string) error {
				return c.DeleteClusterDefaults(ctx, uid)
			},
		},
		{
			name:         "DeleteClusterDefaults not found",
			uid:          "missing-uid",
			path:         "/clusterdefaultss/missing-uid",
			responseBody: errorResponseJSON,
			statusCode:   http.StatusNotFound,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string) error {
				return c.DeleteClusterDefaults(ctx, uid)
			},
		},
		{
			name:         "DeleteGroup success",
			uid:          "group-uid-1",
			path:         "/groups/group-uid-1",
			responseBody: `{"message":"deleted","uid":"group-uid-1"}`,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string) error {
				return c.DeleteGroup(ctx, uid)
			},
		},
		{
			name:         "DeleteGroup not found",
			uid:          "missing-uid",
			path:         "/groups/missing-uid",
			responseBody: errorResponseJSON,
			statusCode:   http.StatusNotFound,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string) error {
				return c.DeleteGroup(ctx, uid)
			},
		},
		{
			name:         "DeleteInstanceInfo success",
			uid:          "ii-uid-1",
			path:         "/instanceinfos/ii-uid-1",
			responseBody: `{"message":"deleted","uid":"ii-uid-1"}`,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string) error {
				return c.DeleteInstanceInfo(ctx, uid)
			},
		},
		{
			name:         "DeleteWireGuardPeer success",
			uid:          "wg-uid-1",
			path:         "/wireguardpeers/wg-uid-1",
			responseBody: `{"message":"deleted","uid":"wg-uid-1"}`,
			statusCode:   http.StatusOK,
			expectError:  false,
			testFunc: func(c *client.Client, ctx context.Context, uid string) error {
				return c.DeleteWireGuardPeer(ctx, uid)
			},
		},
		{
			name:         "DeleteClusterDefaults server error",
			uid:          "cd-uid-1",
			path:         "/clusterdefaultss/cd-uid-1",
			responseBody: serverErrorJSON,
			statusCode:   http.StatusInternalServerError,
			expectError:  true,
			testFunc: func(c *client.Client, ctx context.Context, uid string) error {
				return c.DeleteClusterDefaults(ctx, uid)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				verifyRequestBasics(t, r, "DELETE", tt.path)
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			err := tt.testFunc(c, context.Background(), tt.uid)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// HTTP Error Handling Tests

func TestHTTPErrorResponses(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrMsg string
	}{
		{
			name:           "400 Bad Request",
			statusCode:     http.StatusBadRequest,
			responseBody:   validationErrorJSON,
			expectedErrMsg: "validation failed",
		},
		{
			name:           "401 Unauthorized",
			statusCode:     http.StatusUnauthorized,
			responseBody:   unauthorizedErrorJSON,
			expectedErrMsg: "unauthorized",
		},
		{
			name:           "403 Forbidden",
			statusCode:     http.StatusForbidden,
			responseBody:   forbiddenErrorJSON,
			expectedErrMsg: "forbidden",
		},
		{
			name:           "404 Not Found",
			statusCode:     http.StatusNotFound,
			responseBody:   errorResponseJSON,
			expectedErrMsg: "resource not found",
		},
		{
			name:           "409 Conflict",
			statusCode:     http.StatusConflict,
			responseBody:   conflictErrorJSON,
			expectedErrMsg: "already exists",
		},
		{
			name:           "500 Internal Server Error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   serverErrorJSON,
			expectedErrMsg: "internal server error",
		},
		{
			name:           "503 Service Unavailable",
			statusCode:     http.StatusServiceUnavailable,
			responseBody:   serviceUnavailableJSON,
			expectedErrMsg: "service unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			_, err := c.GetHealth(context.Background())
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.expectedErrMsg))
		})
	}
}

func TestNetworkErrors(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() *client.Client
		expectError bool
	}{
		{
			name: "connection refused",
			setupFunc: func() *client.Client {
				// Use a port that's not listening
				c, _ := client.NewClient("http://localhost:9999", nil, client.DefaultLogger())
				return c
			},
			expectError: true,
		},
		{
			name: "context timeout",
			setupFunc: func() *client.Client {
				// Server that never responds
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second)
				}))
				t.Cleanup(server.Close)
				c, _ := client.NewClient(server.URL, nil, client.DefaultLogger())
				return c
			},
			expectError: true,
		},
		{
			name: "context cancellation",
			setupFunc: func() *client.Client {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second)
				}))
				t.Cleanup(server.Close)
				c, _ := client.NewClient(server.URL, nil, client.DefaultLogger())
				return c
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.setupFunc()

			var ctx context.Context
			var cancel context.CancelFunc

			if strings.Contains(tt.name, "timeout") {
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
			} else if strings.Contains(tt.name, "cancellation") {
				ctx, cancel = context.WithCancel(context.Background())
				// Cancel immediately
				cancel()
			} else {
				ctx = context.Background()
			}

			_, err := c.GetHealth(ctx)
			if tt.expectError {
				require.Error(t, err)
			}
		})
	}
}

// Request Header Tests

func TestRequestHeaders(t *testing.T) {
	tests := []struct {
		name                string
		setupClient         func(string) *client.Client
		expectContentType   string
		expectAccept        string
		expectAuthorization string
	}{
		{
			name: "default headers without version or token",
			setupClient: func(url string) *client.Client {
				c, _ := client.NewClient(url, nil, client.DefaultLogger())
				return c
			},
			expectContentType:   "application/json",
			expectAccept:        "application/json",
			expectAuthorization: "",
		},
		{
			name: "headers with version",
			setupClient: func(url string) *client.Client {
				c, _ := client.NewClient(url, nil, client.DefaultLogger())
				return c.WithVersion("v1")
			},
			expectContentType:   "application/json;version=v1",
			expectAccept:        "application/json;version=v1",
			expectAuthorization: "",
		},
		{
			name: "headers with bearer token",
			setupClient: func(url string) *client.Client {
				c, _ := client.NewClient(url, nil, client.DefaultLogger())
				return c.WithBearerToken("test-token-123")
			},
			expectContentType:   "application/json",
			expectAccept:        "application/json",
			expectAuthorization: "Bearer test-token-123",
		},
		{
			name: "headers with both version and token",
			setupClient: func(url string) *client.Client {
				c, _ := client.NewClient(url, nil, client.DefaultLogger())
				return c.WithVersion("v2").WithBearerToken("test-token-456")
			},
			expectContentType:   "application/json;version=v2",
			expectAccept:        "application/json;version=v2",
			expectAuthorization: "Bearer test-token-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify Accept header
				verifyAcceptHeader(t, r, tt.expectAccept)

				// Verify Authorization header
				if tt.expectAuthorization != "" {
					require.Equal(t, tt.expectAuthorization, r.Header.Get("Authorization"))
				} else {
					require.Empty(t, r.Header.Get("Authorization"))
				}

				respondJSON(w, http.StatusOK, healthResponseJSON)
			}))
			defer server.Close()

			c := tt.setupClient(server.URL)
			_, err := c.GetHealth(context.Background())
			require.NoError(t, err)
		})
	}
}

func TestRequestBodyMarshaling(t *testing.T) {
	tests := []struct {
		name            string
		createRequest   interface{}
		validateRequest func(*testing.T, string)
	}{
		{
			name: "ClusterDefaults request marshals correctly",
			createRequest: client.CreateClusterDefaultsRequest{
				Metadata: fabrica.Metadata{Name: "test-cluster"},
				Spec: v1.ClusterDefaultsSpec{
					BaseURL:       "http://test.local",
					CloudProvider: "OpenCHAMI",
					ClusterName:   "testcluster",
				},
			},
			validateRequest: func(t *testing.T, body string) {
				require.Contains(t, body, `"name":"test-cluster"`)
				require.Contains(t, body, `"base_url":"http://test.local"`)
				require.Contains(t, body, `"cloud_provider":"OpenCHAMI"`)
			},
		},
		{
			name: "Group request with template marshals correctly",
			createRequest: client.CreateGroupRequest{
				Metadata: fabrica.Metadata{Name: "compute"},
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{hostname}}",
					MetaData: map[string]string{"hostname": "test-host"},
				},
			},
			validateRequest: func(t *testing.T, body string) {
				require.Contains(t, body, `"name":"compute"`)
				require.Contains(t, body, `"template":"#cloud-config\nhostname: {{hostname}}"`)
				require.Contains(t, body, `"hostname":"test-host"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := readRequestBody(t, r)
				require.NotEmpty(t, body)

				// Verify it's valid JSON
				var jsonCheck map[string]interface{}
				err := json.Unmarshal([]byte(body), &jsonCheck)
				require.NoError(t, err)

				// Custom validation
				if tt.validateRequest != nil {
					tt.validateRequest(t, body)
				}

				// Return appropriate response based on request type
				switch tt.createRequest.(type) {
				case client.CreateClusterDefaultsRequest:
					respondJSON(w, http.StatusCreated, clusterDefaultsJSON)
				case client.CreateGroupRequest:
					respondJSON(w, http.StatusCreated, groupValidJSON)
				}
			}))
			defer server.Close()

			c, err := client.NewClient(server.URL, nil, client.DefaultLogger())
			require.NoError(t, err)

			// Execute create based on type
			switch req := tt.createRequest.(type) {
			case client.CreateClusterDefaultsRequest:
				_, err = c.CreateClusterDefaults(context.Background(), req)
			case client.CreateGroupRequest:
				_, err = c.CreateGroup(context.Background(), req)
			}
			require.NoError(t, err)
		})
	}
}

// Response Parsing Tests

func TestResponseParsing(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		testFunc     func(*client.Client, context.Context) (interface{}, error)
		validateFunc func(*testing.T, interface{})
	}{
		{
			name:         "valid JSON with all fields",
			responseBody: clusterDefaultsListJSON,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetClusterDefaultss(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.ClusterDefaults)
				require.Len(t, list, 1)
			},
		},
		{
			name: "valid JSON with extra unknown fields",
			responseBody: `[{
				"metadata": {"uid": "cd-uid-1", "name": "test-cluster"},
				"spec": {"base_url": "http://test.local"},
				"status": {},
				"extraField": "should be ignored"
			}]`,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetClusterDefaultss(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.ClusterDefaults)
				require.Len(t, list, 1)
			},
		},
		{
			name:         "empty array response",
			responseBody: `[]`,
			testFunc: func(c *client.Client, ctx context.Context) (interface{}, error) {
				return c.GetGroups(ctx)
			},
			validateFunc: func(t *testing.T, result interface{}) {
				list := result.([]v1.Group)
				require.Len(t, list, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, http.StatusOK, tt.responseBody)
			})
			defer server.Close()

			result, err := tt.testFunc(c, context.Background())
			require.NoError(t, err)
			if tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

func TestErrorResponseParsing(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrMsg string
	}{
		{
			name:           "standard error format",
			statusCode:     http.StatusNotFound,
			responseBody:   errorResponseJSON,
			expectedErrMsg: "resource not found",
		},
		{
			name:           "error with extra fields",
			statusCode:     http.StatusBadRequest,
			responseBody:   `{"error": "validation failed", "details": {"field": "name"}}`,
			expectedErrMsg: "validation failed",
		},
		{
			name:           "malformed JSON error response",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{invalid json}`,
			expectedErrMsg: "500",
		},
		{
			name:           "empty error response body",
			statusCode:     http.StatusBadRequest,
			responseBody:   ``,
			expectedErrMsg: "400",
		},
		{
			name:           "HTML error response",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `<html><body>Internal Server Error</body></html>`,
			expectedErrMsg: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(tt.responseBody, "<html>") {
					w.Header().Set("Content-Type", "text/html")
				} else {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			})
			defer server.Close()

			_, err := c.GetHealth(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectedErrMsg)
		})
	}
}

// Context Handling Tests

func TestContextCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		// Wait longer than the test will allow
		time.Sleep(5 * time.Second)
		respondJSON(w, http.StatusOK, healthResponseJSON)
	}))
	defer server.Close()

	c, err := client.NewClient(server.URL, nil, client.DefaultLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	// Start request in goroutine
	errChan := make(chan error, 1)
	go func() {
		_, err := c.GetHealth(ctx)
		errChan <- err
	}()

	// Wait for request to start, then cancel
	<-requestStarted
	cancel()

	// Should get context cancellation error
	err = <-errChan
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
}

func TestContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay longer than timeout
		time.Sleep(200 * time.Millisecond)
		respondJSON(w, http.StatusOK, healthResponseJSON)
	}))
	defer server.Close()

	c, err := client.NewClient(server.URL, nil, client.DefaultLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = c.GetHealth(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context deadline exceeded")
}

// Group Template Validation Unit Tests

func TestGroupTemplateValidationUnit(t *testing.T) {
	tests := []struct {
		name         string
		operation    string
		request      interface{}
		responseBody string
		statusCode   int
		expectError  bool
		validateFunc func(*testing.T, interface{})
	}{
		{
			name:      "create group with valid template",
			operation: "create",
			request: client.CreateGroupRequest{
				Metadata: fabrica.Metadata{Name: "compute"},
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{hostname}}",
					MetaData: map[string]string{"hostname": "test-host"},
				},
			},
			responseBody: groupValidJSON,
			statusCode:   http.StatusCreated,
			expectError:  false,
			validateFunc: func(t *testing.T, result interface{}) {
				group := result.(*v1.Group)
				require.True(t, group.Status.Valid)
			},
		},
		{
			name:      "create group with missing template variable",
			operation: "create",
			request: client.CreateGroupRequest{
				Metadata: fabrica.Metadata{Name: "invalid"},
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{missing_var}}",
					MetaData: map[string]string{"hostname": "test-host"},
				},
			},
			responseBody: validationErrorJSON,
			statusCode:   http.StatusBadRequest,
			expectError:  true,
			validateFunc: nil,
		},
		{
			name:      "create group with invalid YAML after rendering",
			operation: "create",
			request: client.CreateGroupRequest{
				Metadata: fabrica.Metadata{Name: "invalid-yaml"},
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{hostname}}\nfoo: [",
					MetaData: map[string]string{"hostname": "test-host"},
				},
			},
			responseBody: validationErrorJSON,
			statusCode:   http.StatusBadRequest,
			expectError:  true,
			validateFunc: nil,
		},
		{
			name:      "update group with invalid template",
			operation: "update",
			request: client.UpdateGroupRequest{
				Spec: v1.GroupSpec{
					Template: "#cloud-config\nhostname: {{undefined_var}}",
					MetaData: map[string]string{"hostname": "test-host"},
				},
			},
			responseBody: validationErrorJSON,
			statusCode:   http.StatusBadRequest,
			expectError:  true,
			validateFunc: nil,
		},
		{
			name:      "group status reflects validation result",
			operation: "create",
			request: client.CreateGroupRequest{
				Metadata: fabrica.Metadata{Name: "validated"},
				Spec: v1.GroupSpec{
					Template: "#cloud-config\npackages:\n  - {{package}}",
					MetaData: map[string]string{"package": "gcc"},
				},
			},
			responseBody: groupValidJSON,
			statusCode:   http.StatusCreated,
			expectError:  false,
			validateFunc: func(t *testing.T, result interface{}) {
				group := result.(*v1.Group)
				require.True(t, group.Status.Valid)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				body := readRequestBody(t, r)
				require.NotEmpty(t, body)

				// Verify request contains template and metaData
				var reqData map[string]interface{}
				err := json.Unmarshal([]byte(body), &reqData)
				require.NoError(t, err)

				respondJSON(w, tt.statusCode, tt.responseBody)
			})
			defer server.Close()

			var result interface{}
			var err error

			switch tt.operation {
			case "create":
				result, err = c.CreateGroup(context.Background(), tt.request.(client.CreateGroupRequest))
			case "update":
				result, err = c.UpdateGroup(context.Background(), "group-uid-1", tt.request.(client.UpdateGroupRequest))
			}

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
			}
		})
	}
}

// Test malformed response handling
func TestMalformedResponseHandling(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		contentType  string
	}{
		{
			name:         "malformed JSON",
			responseBody: `{"invalid": json}`,
			contentType:  "application/json",
		},
		{
			name:         "truncated JSON",
			responseBody: `{"metadata":{"uid":"test"`,
			contentType:  "application/json",
		},
		{
			name:         "non-JSON response",
			responseBody: `This is plain text`,
			contentType:  "text/plain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.responseBody))
			})
			defer server.Close()

			_, err := c.GetClusterDefaultss(context.Background())
			require.Error(t, err)
		})
	}
}

// Test request with nil body doesn't set Content-Type
func TestRequestWithNilBody(t *testing.T) {
	c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// GET requests shouldn't have Content-Type
		if r.Method == "GET" {
			// Content-Type may be absent or empty for GET
			contentType := r.Header.Get("Content-Type")
			if contentType != "" {
				// If present, should not be for a body
				require.NotContains(t, contentType, "application/json")
			}
		}
		respondJSON(w, http.StatusOK, healthResponseJSON)
	})
	defer server.Close()

	_, err := c.GetHealth(context.Background())
	require.NoError(t, err)
}

// Test large response handling
func TestLargeResponseHandling(t *testing.T) {
	// Create a large list of resources
	var resources []string
	for i := 0; i < 100; i++ {
		resources = append(resources, clusterDefaultsJSON)
	}
	largeResponse := "[" + strings.Join(resources, ",") + "]"

	c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, largeResponse)
	})
	defer server.Close()

	result, err := c.GetClusterDefaultss(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 100)
}

// Test concurrent requests
func TestConcurrentRequests(t *testing.T) {
	c, server := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, healthResponseJSON)
	})
	defer server.Close()

	const numRequests = 10
	errChan := make(chan error, numRequests)

	// Launch concurrent requests
	for i := 0; i < numRequests; i++ {
		go func() {
			_, err := c.GetHealth(context.Background())
			errChan <- err
		}()
	}

	// Collect results
	for i := 0; i < numRequests; i++ {
		err := <-errChan
		require.NoError(t, err)
	}
}
