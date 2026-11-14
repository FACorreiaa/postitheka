package httpapi

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FACorreiaa/postitheka/internal/ratelimit"
)

// Limits are the API's rate limits (OPS-002), each per minute. They bound what
// one account or one address can cost: a post fans out to every chosen
// platform, an upload reservation mints a presigned URL into the bucket, and a
// request with a bad token is a database lookup anyone can cause.
type Limits struct {
	// PostsPerMinute is per user. A post is one request however many
	// destinations it has, so this is far above what a person does by hand.
	PostsPerMinute int
	// UploadsPerMinute is per user. One reservation per file, and a post
	// holds up to twenty, so it is a multiple of the post limit.
	UploadsPerMinute int
	// UnauthenticatedPerMinute is per address, and counts only requests that
	// fail authentication: a signed-in user is never held up by a neighbour
	// behind the same address.
	UnauthenticatedPerMinute int
	// PublicPerMinute is per address, for the routes that take no token at
	// all (the OAuth callback and the sign-in page's config).
	PublicPerMinute int
}

// DefaultLimits are set by judgement, not measurement: nobody but the owner
// has used the API yet. Revisit them against real traffic.
var DefaultLimits = Limits{
	PostsPerMinute:           30,
	UploadsPerMinute:         120,
	UnauthenticatedPerMinute: 30,
	PublicPerMinute:          120,
}

type limiters struct {
	posts   *ratelimit.Limiter
	uploads *ratelimit.Limiter
	failed  *ratelimit.Limiter
}

func newLimiters(l Limits) limiters {
	return limiters{
		posts:   ratelimit.New(l.PostsPerMinute, time.Minute),
		uploads: ratelimit.New(l.UploadsPerMinute, time.Minute),
		failed:  ratelimit.New(l.UnauthenticatedPerMinute, time.Minute),
	}
}

// AddressLimit refuses an address that sends more than perMinute requests to
// the routes it wraps. For routes outside the spec, which have no user to
// count against.
func AddressLimit(perMinute int) func(http.Handler) http.Handler {
	limiter := ratelimit.New(perMinute, time.Minute)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ok, wait := limiter.Allow(clientAddress(r)); !ok {
				writeTooManyRequests(w, wait)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientAddress is the address to count a request against.
//
// Behind the ingress every connection comes from the proxy, which appends the
// address it saw to X-Forwarded-For. That last entry is the only part of the
// header the client did not write, so it is the one used: counting by the
// first entry, or by a header such as X-Real-IP, would let a caller pick a
// fresh address for every request and never be limited. With no header (local
// development, a test) it is the connection's own address.
func clientAddress(r *http.Request) string {
	if forwarded := r.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
		parts := strings.Split(forwarded[len(forwarded)-1], ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
			return last
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func retryAfterSeconds(wait time.Duration) int {
	return int(math.Ceil(wait.Seconds()))
}

const tooManyMessage = "too many requests; wait and try again"

// tooManyRequests is the 429 for a spec operation; the generated visitor
// writes the Retry-After header from it.
func tooManyRequests(wait time.Duration) TooManyRequestsJSONResponse {
	return TooManyRequestsJSONResponse{
		Body:    Error{Message: tooManyMessage},
		Headers: TooManyRequestsResponseHeaders{RetryAfter: retryAfterSeconds(wait)},
	}
}

// writeTooManyRequests is the same answer from middleware, where there is no
// generated response to return.
func writeTooManyRequests(w http.ResponseWriter, wait time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(wait)))
	writeError(w, http.StatusTooManyRequests, tooManyMessage)
}
