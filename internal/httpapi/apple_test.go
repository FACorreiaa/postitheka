package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FACorreiaa/postitheka/internal/service"
)

// The success path and every refusal of a token are in the service's tests; this checks the
// answers the handler adds.
func TestAppleSignInAnswers(t *testing.T) {
	off := AppleSignIn(service.New(service.Deps{}), slog.New(slog.DiscardHandler))
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"not json", "{", http.StatusBadRequest},
		{"no token", `{"nonce":"n"}`, http.StatusBadRequest},
		{"no nonce", `{"identity_token":"t"}`, http.StatusBadRequest},
		{"not configured", `{"identity_token":"t","nonce":"n","client":"ios"}`, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			off(rec, httptest.NewRequest(http.MethodPost, "/auth/apple", strings.NewReader(tc.body)))
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tc.want, rec.Body)
			}
		})
	}
}
