package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/FACorreiaa/postitheka/internal/service"
)

// appleSignIn is the body of POST /auth/apple.
type appleSignIn struct {
	// IdentityToken is the JWT Apple handed the client.
	IdentityToken string `json:"identity_token"`
	// Nonce is the random string the client made; Apple was given its SHA-256.
	Nonce string `json:"nonce"`
	// Client is "ios" or "web"; it names the token, so a person can tell their sessions apart.
	Client string `json:"client"`
}

// AppleSignIn serves POST /auth/apple (AUTH-001): an Apple identity token in, a Postitheka API
// token out. Outside the spec because auth wraps every spec route, and this is how a person
// gets their first token. Clients call it with plain HTTP rather than the generated client.
func AppleSignIn(svc *service.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body appleSignIn
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil || body.IdentityToken == "" || body.Nonce == "" {
			writeError(w, http.StatusBadRequest, "identity_token and nonce are required")
			return
		}
		name := "apple-web"
		if body.Client == "ios" {
			name = "apple-ios"
		}
		token, created, err := svc.SignInWithApple(r.Context(), body.IdentityToken, body.Nonce, name)
		var bad *service.BadRequest
		switch {
		case errors.Is(err, service.ErrAppleOff):
			writeError(w, http.StatusServiceUnavailable, "Sign in with Apple is not set up on this server")
		case errors.Is(err, service.ErrUnauthenticated):
			writeError(w, http.StatusUnauthorized, "Apple's sign-in could not be verified; try again")
		case errors.As(err, &bad):
			writeError(w, http.StatusBadRequest, bad.Message)
		case err != nil:
			log.Error("sign in with apple", slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, "internal error")
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(map[string]any{"token": token, "created": created})
		}
	}
}
