// Package cache defines runtime-neutral contracts for caching complete raw HTTP responses.
package cache

import (
	"context"
	"net/http"
	"time"
)

// Entry is one complete raw response body stored by a cache.
//
// Store implementations must preserve CreatedAt and Body without interpreting
// the response bytes or provider-specific data.
type Entry struct {
	// CreatedAt records when the cache policy accepted Body as a complete entry.
	CreatedAt time.Time
	// Body contains the exact raw response bytes, including an empty body when valid.
	Body []byte
}

// Store persists entries under opaque keys.
//
// Implementations must report a missing key with found set to false and must
// honor context cancellation while reading or writing.
type Store interface {
	// Get retrieves the entry for key, if one exists.
	Get(ctx context.Context, key string) (entry Entry, found bool, err error)
	// Put replaces the entry for key with entry.
	Put(ctx context.Context, key string, entry Entry) error
}

// Request describes one cache-aware HTTP request.
//
// Headers must contain only representation-affecting non-secret headers.
// Credential is optional and contains the exact bytes selected for this GET;
// the cache policy uses those bytes for identity but never sends them itself.
type Request struct {
	// Namespace identifies the source and endpoint family owning the request.
	Namespace string
	// Method is the HTTP method used by the deferred network operation.
	Method string
	// URL is the complete request URL, including query parameters.
	URL string
	// Headers contains representation-affecting non-secret request headers.
	Headers http.Header
	// Credential contains the optional request-scoped credential bytes.
	Credential []byte
	// TTL is the positive duration for which a stored entry remains fresh.
	TTL time.Duration
	// Refresh bypasses cache reads for this request when true.
	Refresh bool
	// Network performs the live request only when cache policy requires it.
	Network DeferredNetwork
}

// Result contains the response body and cache metadata returned to a source.
//
// Warnings retain storage problems that do not prevent a successful live
// response, in the order in which the cache policy observed them.
type Result struct {
	// Body contains the complete raw response body.
	Body []byte
	// StatusCode is the HTTP status associated with Body; a cache hit uses 200.
	StatusCode int
	// FromCache reports whether Body came from a stored entry instead of the network.
	FromCache bool
	// Warnings contains nonfatal cache diagnostics in observation order.
	Warnings []Warning
}

// Warning describes one nonfatal cache-storage failure.
//
// Operation identifies the cache operation, and Err preserves the underlying
// failure for structured handling by the application boundary.
type Warning struct {
	// Operation identifies the cache operation that produced the warning.
	Operation string
	// Err is the underlying nonfatal storage error.
	Err error
}

// DeferredNetwork performs one live HTTP request when cache policy invokes it.
//
// The function owns request construction and authentication. It returns the
// live response or the transport error without changing either result.
type DeferredNetwork func(ctx context.Context) (*http.Response, error)

// Retriever performs cache-aware HTTP retrieval for one request.
//
// Implementations own key derivation, freshness checks, refresh behavior, and
// Store access before invoking Request.Network when a live response is needed.
type Retriever interface {
	// Retrieve returns a cached or live response, ordered nonfatal warnings, or an error.
	Retrieve(ctx context.Context, request Request) (Result, error)
}
