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
// answers the handlers add.
func TestSignInAnswers(t *testing.T) {
	svc := service.New(service.Deps{})
	log := slog.New(slog.DiscardHandler)
	for path, h := range map[string]http.HandlerFunc{"/auth/apple": AppleSignIn(svc, log), "/auth/google": GoogleSignIn(svc, log)} {
		for _, tc := range []struct {
			name, body string
			want       int
		}{
			{"not json", "{", http.StatusBadRequest},
			{"no token", `{"nonce":"n"}`, http.StatusBadRequest},
			{"no nonce", `{"identity_token":"t"}`, http.StatusBadRequest},
			{"not configured", `{"identity_token":"t","nonce":"n","client":"ios"}`, http.StatusServiceUnavailable},
			{"google's field name", `{"id_token":"t","nonce":"n","client":"web"}`, http.StatusServiceUnavailable},
		} {
			t.Run(path+" "+tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				h(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.body)))
				if rec.Code != tc.want {
					t.Fatalf("status %d, want %d (%s)", rec.Code, tc.want, rec.Body)
				}
			})
		}
	}
}
