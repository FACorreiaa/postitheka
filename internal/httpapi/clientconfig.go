package httpapi

import (
	"encoding/json"
	"net/http"
)

// clientConfig is what the web app needs before anyone signs in. Nothing in it
// is secret: it goes to every visitor.
type clientConfig struct {
	// Mascot3DModelURL is null when unset, and the page shows the flat mascot.
	Mascot3DModelURL *string `json:"mascot3dModelUrl"`
	// AppleWebClientID is the Services ID the page signs in to Apple with (AUTH-001). Null
	// hides the Sign in with Apple button.
	AppleWebClientID *string `json:"appleWebClientId"`
}

// ClientConfig serves GET /client-config. It is outside the spec because auth
// wraps every spec route and the sign-in page has no token yet.
func ClientConfig(mascot3DModelURL, appleWebClientID string) http.HandlerFunc {
	var body clientConfig
	if mascot3DModelURL != "" {
		body.Mascot3DModelURL = &mascot3DModelURL
	}
	if appleWebClientID != "" {
		body.AppleWebClientID = &appleWebClientID
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		// A struct of string pointers always marshals.
		panic(err)
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Set per environment, so a cached copy must not outlive a change of it.
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(encoded)
	}
}
