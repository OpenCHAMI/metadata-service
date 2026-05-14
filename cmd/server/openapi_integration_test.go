// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// TestOpenAPIJsonEndpoint verifies /openapi.json returns valid OpenAPI spec
func TestOpenAPIJsonEndpoint(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Register the public endpoints
	r.Group(func(public chi.Router) {
		public.Get("/health", healthHandler)
		public.Get("/openapi.json", ServeOpenAPISpec)
		public.Get("/docs", ServeSwaggerUI)
	})

	// Create test server
	server := httptest.NewServer(r)
	defer server.Close()

	// Make request to /openapi.json
	resp, err := http.Get(server.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("failed to request /openapi.json: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("failed to close /openapi.json response body: %v", closeErr)
		}
	}()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Check content type
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type: application/json, got %s", ct)
	}

	// Parse response as OpenAPI spec
	var spec openapi3.T
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("failed to decode OpenAPI spec: %v", err)
	}

	// Validate spec structure
	if spec.OpenAPI != "3.0.0" {
		t.Fatalf("expected OpenAPI 3.0.0, got %s", spec.OpenAPI)
	}
	if spec.Info == nil {
		t.Fatal("expected Info object, got nil")
	}
	if spec.Paths == nil {
		t.Fatal("expected Paths object, got nil")
	}

	// Verify that the spec includes the documented meta-endpoints
	requiredPaths := []string{"/health", "/openapi.json", "/docs"}
	for _, path := range requiredPaths {
		if spec.Paths.Find(path) == nil {
			t.Fatalf("expected path %s to be documented in spec, but not found", path)
		}
	}

	// Verify /openapi.json and /docs are properly documented
	openAPIPath := spec.Paths.Find("/openapi.json")
	if openAPIPath == nil || openAPIPath.Get == nil {
		t.Fatal("expected /openapi.json GET operation in spec")
	}
	if openAPIPath.Get.OperationID != "getOpenAPISpec" {
		t.Fatalf("expected operationId getOpenAPISpec, got %s", openAPIPath.Get.OperationID)
	}

	docsPath := spec.Paths.Find("/docs")
	if docsPath == nil || docsPath.Get == nil {
		t.Fatal("expected /docs GET operation in spec")
	}
	if docsPath.Get.OperationID != "getSwaggerUI" {
		t.Fatalf("expected operationId getSwaggerUI, got %s", docsPath.Get.OperationID)
	}
}

// TestDocsEndpoint verifies /docs returns valid HTML
func TestDocsEndpoint(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Register the public endpoints
	r.Group(func(public chi.Router) {
		public.Get("/health", healthHandler)
		public.Get("/openapi.json", ServeOpenAPISpec)
		public.Get("/docs", ServeSwaggerUI)
	})

	// Create test server
	server := httptest.NewServer(r)
	defer server.Close()

	// Make request to /docs
	resp, err := http.Get(server.URL + "/docs")
	if err != nil {
		t.Fatalf("failed to request /docs: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("failed to close /docs response body: %v", closeErr)
		}
	}()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Check content type
	ct := resp.Header.Get("Content-Type")
	if ct != "text/html" {
		t.Fatalf("expected Content-Type: text/html, got %s", ct)
	}

	// Read body and verify it contains HTML
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "<!DOCTYPE html>") {
		t.Fatal("expected HTML DOCTYPE, not found in response")
	}
	if !strings.Contains(bodyStr, "swagger-ui") {
		t.Fatal("expected swagger-ui reference, not found in response")
	}
	if !strings.Contains(bodyStr, "/openapi.json") {
		t.Fatal("expected /openapi.json URL reference, not found in response")
	}
}

// TestOpenAPISpecCompleteness verifies that required paths are documented
func TestOpenAPISpecCompleteness(t *testing.T) {
	spec := GenerateOpenAPISpec()

	// Verify spec basic properties
	if spec.OpenAPI != "3.0.0" {
		t.Fatalf("expected OpenAPI 3.0.0, got %s", spec.OpenAPI)
	}

	// Check for service endpoints
	serviceEndpoints := []string{"/health", "/openapi.json", "/docs"}
	for _, endpoint := range serviceEndpoints {
		pathItem := spec.Paths.Find(endpoint)
		if pathItem == nil {
			t.Fatalf("endpoint %s not found in spec", endpoint)
		}
		if pathItem.Get == nil {
			t.Fatalf("endpoint %s has no GET operation", endpoint)
		}
		if len(pathItem.Get.Tags) == 0 {
			t.Fatalf("endpoint %s has no tags", endpoint)
		}
		if pathItem.Get.Tags[0] != "Service" {
			t.Fatalf("endpoint %s should have 'Service' tag, got %s", endpoint, pathItem.Get.Tags[0])
		}
	}

	// Verify responses are documented
	if spec.Paths.Find("/openapi.json").Get.Responses == nil {
		t.Fatal("/openapi.json should have responses documented")
	}
	if spec.Paths.Find("/docs").Get.Responses == nil {
		t.Fatal("/docs should have responses documented")
	}
}
