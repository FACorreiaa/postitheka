package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/FACorreiaa/postitheka/internal/service"
)

// signInBody is the body of POST /auth/apple and POST /auth/google.
type signInBody struct {
	// IdentityToken is the JWT the provider handed the client. Apple calls it an identity token,
	// Google an ID token; either name is read.
	IdentityToken string `json:"identity_token"`
	IDToken       string `json:"id_token"`
	// Nonce is the random string the client made; the provider was given its SHA-256.
	Nonce string `json:"nonce"`
	// Client is "ios" or "web"; it names the token, so a person can tell their sessions apart.
	Client string `json:"client"`
}

// signInFunc is Service.SignInWithApple or Service.SignInWithGoogle.
type signInFunc func(ctx context.Context, idToken, rawNonce, client string) (string, bool, error)

// AppleSignIn serves POST /auth/apple (AUTH-001).
func AppleSignIn(svc *service.Service, log *slog.Logger) http.HandlerFunc {
	return signIn("apple", "Apple", svc.SignInWithApple, log)
}

// GoogleSignIn serves POST /auth/google (AUTH-002).
func GoogleSignIn(svc *service.Service, log *slog.Logger) http.HandlerFunc {
	return signIn("google", "Google", svc.SignInWithGoogle, log)
}

// signIn trades a provider's ID token for a Postitheka API token. Outside the spec because auth
// wraps every spec route, and this is how a person gets their first token. Clients call it with
// plain HTTP rather than the generated client.
func signIn(slug, name string, do signInFunc, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body signInBody
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body)
		token := body.IdentityToken
		if token == "" {
			token = body.IDToken
		}
		if err != nil || token == "" || body.Nonce == "" {
			writeError(w, http.StatusBadRequest, "an ID token and a nonce are required")
			return
		}
		client := slug + "-web"
		if body.Client == "ios" {
			client = slug + "-ios"
		}
		apiToken, created, err := do(r.Context(), token, body.Nonce, client)
		var bad *service.BadRequest
		switch {
		case errors.Is(err, service.ErrProviderOff):
			writeError(w, http.StatusServiceUnavailable, "Sign in with "+name+" is not set up on this server")
		case errors.Is(err, service.ErrUnauthenticated):
			writeError(w, http.StatusUnauthorized, name+"'s sign-in could not be verified; try again")
		case errors.As(err, &bad):
			writeError(w, http.StatusBadRequest, bad.Message)
		case err != nil:
			log.Error("sign in", slog.String("provider", slug), slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, "internal error")
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(map[string]any{"token": apiToken, "created": created})
		}
	}
}
