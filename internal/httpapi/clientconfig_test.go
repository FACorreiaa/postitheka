package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientConfig(t *testing.T) {
	for _, tc := range []struct {
		name, url, apple, google, want string
	}{
		{"unset means the flat mascot and no sign-in buttons", "", "", "", `{"mascot3dModelUrl":null,"appleWebClientId":null,"googleWebClientId":null}`},
		{"set is passed through", "https://cdn.example/mascot3d/v1/mascot.glb", "com.example.web", "123-abc.apps.googleusercontent.com", `{"mascot3dModelUrl":"https://cdn.example/mascot3d/v1/mascot.glb","appleWebClientId":"com.example.web","googleWebClientId":"123-abc.apps.googleusercontent.com"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ClientConfig(tc.url, tc.apple, tc.google)(rec, httptest.NewRequest(http.MethodGet, "/client-config", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			if got := rec.Body.String(); got != tc.want {
				t.Fatalf("body %s, want %s", got, tc.want)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content type %q", ct)
			}
		})
	}
}
