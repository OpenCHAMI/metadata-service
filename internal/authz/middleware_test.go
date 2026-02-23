package authz

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestWrapMiddleware_NoOp(t *testing.T) {
	t.Setenv(EnvAuthzMode, "off")
	os.Unsetenv(EnvAuthzPolicyDir)

	mw := WrapMiddleware()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}
