package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// OPS-002's "done when": exceeding a limit returns 429 with Retry-After.

func asUser(id uuid.UUID) context.Context {
	return context.WithValue(context.Background(), userKey{}, id)
}

// checkTooMany asserts the wire form of a refusal: the status, a Retry-After
// of whole seconds, and the JSON error every other failure uses.
func checkTooMany(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60 (one a minute, bucket just emptied)", got)
	}
	var body Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Message == "" {
		t.Errorf("body %q is not the API's error shape: %v", rec.Body.String(), err)
	}
}

// The handlers refuse before they touch the service, so a Server with no
// service proves a refused request costs nothing: reaching it would panic.
func TestCreatingPostsPastTheLimitIsRefused(t *testing.T) {
	s := &Server{limits: newLimiters(Limits{PostsPerMinute: 1, UploadsPerMinute: 1, UnauthenticatedPerMinute: 1})}
	alice, bob := uuid.New(), uuid.New()
	s.limits.posts.Allow(alice.String()) // alice's one post this minute

	resp, err := s.CreatePost(asUser(alice), CreatePostRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := resp.VisitCreatePostResponse(rec); err != nil {
		t.Fatal(err)
	}
	checkTooMany(t, rec)

	// Per user: bob has spent nothing, so he gets past the limit. What happens
	// next, with no service behind the handler, is not this test's business:
	// only that the answer is not a refusal.
	var bobs CreatePostResponseObject
	func() {
		defer func() { _ = recover() }()
		bobs, _ = s.CreatePost(asUser(bob), CreatePostRequestObject{Body: &CreatePostJSONRequestBody{}})
	}()
	if _, refused := bobs.(CreatePost429JSONResponse); refused {
		t.Error("bob was refused because alice spent her allowance")
	}
	if ok, _ := s.limits.posts.Allow(bob.String()); ok {
		t.Error("bob's request did not count against his own allowance")
	}
}

func TestReservingUploadsPastTheLimitIsRefused(t *testing.T) {
	s := &Server{limits: newLimiters(Limits{PostsPerMinute: 1, UploadsPerMinute: 1, UnauthenticatedPerMinute: 1})}
	alice := uuid.New()
	s.limits.uploads.Allow(alice.String())

	resp, err := s.CreateUpload(asUser(alice), CreateUploadRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := resp.VisitCreateUploadResponse(rec); err != nil {
		t.Fatal(err)
	}
	checkTooMany(t, rec)

	// The two limits are separate: a spent upload allowance leaves posts alone.
	if ok, _ := s.limits.posts.Allow(alice.String()); !ok {
		t.Error("reserving uploads used up the post allowance")
	}
}

func TestFailedAuthenticationIsLimitedByAddress(t *testing.T) {
	s := &Server{limits: newLimiters(Limits{PostsPerMinute: 1, UploadsPerMinute: 1, UnauthenticatedPerMinute: 2})}
	h := s.authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("a request with no token reached the handler")
	}))

	send := func(forwardedFor string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
		req.RemoteAddr = "10.42.0.7:51234" // the ingress, the same for everyone
		req.Header.Set("X-Forwarded-For", forwardedFor)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := range 2 {
		if rec := send("203.0.113.9"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: status = %d, want 401 while under the limit", i+1, rec.Code)
		}
	}
	rec := send("203.0.113.9")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("third failure: status = %d, Retry-After = %q, want 429 with a wait", rec.Code, rec.Header().Get("Retry-After"))
	}

	// A caller cannot buy a new allowance by writing its own header: the
	// proxy appends the address it saw, and only that last entry counts.
	if rec := send("198.51.100.1, 203.0.113.9"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("a forged first entry got status %d, want 429", rec.Code)
	}
	// Someone else, at another address, is unaffected.
	if rec := send("203.0.113.50"); rec.Code != http.StatusUnauthorized {
		t.Errorf("another address got status %d, want 401", rec.Code)
	}
}

func TestAddressLimitOnPublicRoutes(t *testing.T) {
	served := 0
	h := AddressLimit(2)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served++ }))

	send := func(remote string) int {
		req := httptest.NewRequest(http.MethodGet, "/client-config", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// No X-Forwarded-For (local development): the connection's address,
	// whatever port each request came from.
	if a, b, c := send("192.0.2.1:1000"), send("192.0.2.1:2000"), send("192.0.2.1:3000"); a != 200 || b != 200 || c != 429 {
		t.Fatalf("statuses = %d %d %d, want 200 200 429", a, b, c)
	}
	if served != 2 {
		t.Errorf("the handler ran %d times, want 2: a refused request must not reach it", served)
	}
	if got := send("192.0.2.2:1000"); got != 200 {
		t.Errorf("another address got %d, want 200", got)
	}
}

func TestClientAddress(t *testing.T) {
	for _, tc := range []struct {
		name      string
		remote    string
		forwarded []string
		want      string
	}{
		{"no proxy", "192.0.2.1:4000", nil, "192.0.2.1"},
		{"no proxy, IPv6", "[2001:db8::1]:4000", nil, "2001:db8::1"},
		{"one proxy", "10.42.0.7:4000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"client wrote the first entry", "10.42.0.7:4000", []string{"1.2.3.4, 203.0.113.9"}, "203.0.113.9"},
		{"client sent its own header line", "10.42.0.7:4000", []string{"1.2.3.4", "203.0.113.9"}, "203.0.113.9"},
		{"empty header falls back", "10.42.0.7:4000", []string{" "}, "10.42.0.7"},
		{"address without a port", "192.0.2.1", nil, "192.0.2.1"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tc.remote
		for _, v := range tc.forwarded {
			req.Header.Add("X-Forwarded-For", v)
		}
		if got := clientAddress(req); got != tc.want {
			t.Errorf("%s: clientAddress = %q, want %q", tc.name, got, tc.want)
		}
	}
}
