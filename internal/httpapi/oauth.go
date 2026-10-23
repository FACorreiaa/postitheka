package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/service"
)

// OAuthCallback is where platforms send the user back after their consent
// page. It is outside the spec because it is a browser redirect, with no
// bearer token: the state, minted for one user by StartConnect, is what ties
// it to an account. It always answers with a redirect to the client the user
// started from, carrying the outcome as a code, never a platform's message.
func OAuthCallback(svc *service.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		platform := publisher.Platform(chi.URLParam(r, "platform"))
		q := r.URL.Query()
		client, _, err := svc.FinishConnect(r.Context(), platform, q.Get("state"), q.Get("code"), q.Get("error"))

		var failure service.ConnectFailure
		switch {
		case err == nil:
		case errors.As(err, &failure):
		case errors.Is(err, service.ErrNotFound):
			http.NotFound(w, r)
			return
		default:
			log.Error("oauth callback failed", slog.String("platform", string(platform)), slog.Any("error", err))
			failure = service.ConnectFailed
		}
		// The code is single-use and the page is a redirect, but a cached
		// copy of a URL with a code in it is still worth not having.
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, connectReturnURL(client, platform, failure), http.StatusSeeOther)
	}
}

func connectReturnURL(client service.ConnectClient, platform publisher.Platform, failure service.ConnectFailure) string {
	v := url.Values{}
	if client == service.ConnectFromIOS {
		// ASWebAuthenticationSession ends when it sees this scheme.
		if failure != "" {
			v.Set("error", string(failure))
		} else {
			v.Set("platform", string(platform))
		}
		return "postitheka://connected?" + v.Encode()
	}
	// An unknown state has no client; the web app is the safe default.
	if failure != "" {
		v.Set("connect_error", string(failure))
	} else {
		v.Set("connected", string(platform))
	}
	return "/?" + v.Encode()
}
