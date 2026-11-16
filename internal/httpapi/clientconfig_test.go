package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientConfig(t *testing.T) {
	for _, tc := range []struct {
		name, url, apple, want string
	}{
		{"unset means the flat mascot and no Apple button", "", "", `{"mascot3dModelUrl":null,"appleWebClientId":null}`},
		{"set is passed through", "https://cdn.example/mascot3d/v1/mascot.glb", "com.example.web", `{"mascot3dModelUrl":"https://cdn.example/mascot3d/v1/mascot.glb","appleWebClientId":"com.example.web"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ClientConfig(tc.url, tc.apple)(rec, httptest.NewRequest(http.MethodGet, "/client-config", nil))
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
