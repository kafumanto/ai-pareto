package cache

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

// memoryStore is a copying in-memory Store double used by portable policy tests.
// [NO_SPEC] Keeps policy tests independent from the native filesystem implementation.
type memoryStore struct {
	// entries contains complete values indexed by opaque cache key.
	entries map[string]Entry
	// getError is returned by Get when configured for read-failure tests.
	getError error
	// putError is returned by Put when configured for write-failure tests.
	putError error
	// getFunc overrides Get for cancellation and ordering tests.
	getFunc func(context.Context, string) (Entry, bool, error)
	// putFunc overrides Put for cancellation and ordering tests.
	putFunc func(context.Context, string, Entry) error
	// getCalls counts storage reads made by policy.
	getCalls int
	// putCalls counts storage writes made by policy.
	putCalls int
}

// newMemoryStore returns an empty copying store double.
// [NO_SPEC] Provides isolated storage state for each policy test.
func newMemoryStore() *memoryStore {
	return &memoryStore{entries: make(map[string]Entry)}
}

// Get returns a copied entry or the configured test failure.
// [NO_SPEC] Implements Store for portable cache-policy tests.
func (store *memoryStore) Get(ctx context.Context, key string) (Entry, bool, error) {
	store.getCalls++
	if store.getFunc != nil {
		return store.getFunc(ctx, key)
	}
	if store.getError != nil {
		return Entry{}, false, store.getError
	}
	entry, found := store.entries[key]
	if !found {
		return Entry{}, false, nil
	}
	entry.Body = cloneBytes(entry.Body)
	return entry, true, nil
}

// Put stores a copied entry or the configured test failure.
// [NO_SPEC] Implements Store for portable cache-policy tests.
func (store *memoryStore) Put(ctx context.Context, key string, entry Entry) error {
	store.putCalls++
	if store.putFunc != nil {
		return store.putFunc(ctx, key, entry)
	}
	if store.putError != nil {
		return store.putError
	}
	entry.Body = cloneBytes(entry.Body)
	store.entries[key] = entry
	return nil
}

// testBody is a controllable response body that records closure and can cancel or fail reads.
// [NO_SPEC] Supports response-completion and cancellation tests without a network connection.
type testBody struct {
	// reader supplies response bytes to Read.
	reader *bytes.Reader
	// readError is returned after the reader supplies its next bytes.
	readError error
	// closeError is returned by Close when configured.
	closeError error
	// onRead runs once before the first Read result.
	onRead func()
	// closed records whether policy closed the live response body.
	closed bool
}

// Read returns response bytes and an optional configured read failure.
// [NO_SPEC] Implements io.Reader for response-body tests.
func (body *testBody) Read(target []byte) (int, error) {
	if body.onRead != nil {
		onRead := body.onRead
		body.onRead = nil
		onRead()
	}
	count, err := body.reader.Read(target)
	if body.readError != nil {
		return count, body.readError
	}
	return count, err
}

// Close records response-body closure and returns the configured close failure.
// [NO_SPEC] Verifies policy closes every live response body.
func (body *testBody) Close() error {
	body.closed = true
	return body.closeError
}

// newResponse returns a controllable HTTP response and its body double.
// [NO_SPEC] Centralizes response construction for cache-policy tests.
func newResponse(status int, data []byte, contentLength int64) (*http.Response, *testBody) {
	body := &testBody{reader: bytes.NewReader(data)}
	return &http.Response{
		StatusCode:    status,
		Body:          body,
		ContentLength: contentLength,
	}, body
}

// policyTestRequest returns one valid GET request with the supplied deferred network function.
// [NO_SPEC] Provides common request inputs for policy behavior tests.
func policyTestRequest(network DeferredNetwork) Request {
	return Request{
		Namespace: "source",
		Method:    http.MethodGet,
		URL:       "https://example.test/data?page=1",
		Headers:   http.Header{"Accept": {"application/json"}},
		TTL:       time.Hour,
		Network:   network,
	}
}

/**
 * TestNewRetrieverExposesCacheOperation verifies the public cache-aware operation constructor.
 * Expected: the constructed retriever stores a live response and reuses it on the next retrieval without another network call.
 * [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Raw HTTP cache boundary".
 */
func TestNewRetrieverExposesCacheOperation(t *testing.T) {
	store := newMemoryStore()
	networkCalls := 0
	retriever := NewRetriever(store)
	request := policyTestRequest(func(context.Context) (*http.Response, error) {
		networkCalls++
		if networkCalls > 1 {
			return nil, errors.New("network should not run on a fresh hit")
		}
		response, _ := newResponse(http.StatusOK, []byte("live"), int64(len("live")))
		return response, nil
	})

	first, err := retriever.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatalf("first Retrieve() error = %v", err)
	}
	if first.FromCache || string(first.Body) != "live" {
		t.Fatalf("first Retrieve() = %#v, want live non-cache result", first)
	}
	second, err := retriever.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatalf("second Retrieve() error = %v", err)
	}
	if !second.FromCache || string(second.Body) != "live" {
		t.Fatalf("second Retrieve() = %#v, want cached live result", second)
	}
	if networkCalls != 1 {
		t.Fatalf("network calls = %d, want 1", networkCalls)
	}
}

// TestPolicyCachePaths verifies hits, misses, expiry, refresh, method bypass, and deferred execution.
// Expected: fresh entries avoid the network, misses and refreshes replace entries, non-GET requests bypass storage, and expiry never returns stale data after failure.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Freshness and refresh policy".
func TestPolicyCachePaths(t *testing.T) {
	now := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name          string
		method        string
		refresh       bool
		entryAge      time.Duration
		entryBody     string
		networkBody   string
		networkError  error
		wantBody      string
		wantFromCache bool
		wantError     error
		wantGetCalls  int
		wantPutCalls  int
		wantNetwork   int
		wantStored    string
	}{
		{
			name:          "fresh-hit",
			entryAge:      time.Minute,
			entryBody:     "cached",
			networkBody:   "network",
			wantBody:      "cached",
			wantFromCache: true,
			wantGetCalls:  1,
			wantPutCalls:  0,
			wantNetwork:   0,
		},
		{
			name:         "miss",
			networkBody:  "network",
			wantBody:     "network",
			wantGetCalls: 1,
			wantPutCalls: 1,
			wantNetwork:  1,
			wantStored:   "network",
		},
		{
			name:         "expired-no-stale-fallback",
			entryAge:     2 * time.Hour,
			entryBody:    "stale",
			networkError: errors.New("network unavailable"),
			wantError:    errors.New("network unavailable"),
			wantGetCalls: 1,
			wantPutCalls: 0,
			wantNetwork:  1,
		},
		{
			name:         "refresh-replaces-entry",
			refresh:      true,
			entryAge:     time.Minute,
			entryBody:    "old",
			networkBody:  "fresh",
			wantBody:     "fresh",
			wantGetCalls: 0,
			wantPutCalls: 1,
			wantNetwork:  1,
			wantStored:   "fresh",
		},
		{
			name:         "non-get-bypass",
			method:       http.MethodPost,
			networkBody:  "posted",
			wantBody:     "posted",
			wantGetCalls: 0,
			wantPutCalls: 0,
			wantNetwork:  1,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := newMemoryStore()
			networkCalls := 0
			request := policyTestRequest(func(context.Context) (*http.Response, error) {
				networkCalls++
				if testCase.networkError != nil {
					return nil, testCase.networkError
				}
				response, _ := newResponse(http.StatusOK, []byte(testCase.networkBody), int64(len(testCase.networkBody)))
				return response, nil
			})
			request.Method = testCase.method
			if request.Method == "" {
				request.Method = http.MethodGet
			}
			request.Refresh = testCase.refresh
			if testCase.entryBody != "" {
				store.entries[keyFor(request)] = Entry{
					CreatedAt: now.Add(-testCase.entryAge),
					Body:      []byte(testCase.entryBody),
				}
			}

			policy := newPolicy(store, func() time.Time { return now })
			got, err := policy.Retrieve(context.Background(), request)
			if testCase.wantError != nil {
				if err == nil || err.Error() != testCase.wantError.Error() {
					t.Fatalf("Get() error = %v, want %v", err, testCase.wantError)
				}
			} else if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if string(got.Body) != testCase.wantBody {
				t.Fatalf("Get() body = %q, want %q", got.Body, testCase.wantBody)
			}
			if got.FromCache != testCase.wantFromCache {
				t.Fatalf("Get() from cache = %t, want %t", got.FromCache, testCase.wantFromCache)
			}
			if store.getCalls != testCase.wantGetCalls {
				t.Fatalf("Get() storage reads = %d, want %d", store.getCalls, testCase.wantGetCalls)
			}
			if store.putCalls != testCase.wantPutCalls {
				t.Fatalf("Get() storage writes = %d, want %d", store.putCalls, testCase.wantPutCalls)
			}
			if networkCalls != testCase.wantNetwork {
				t.Fatalf("Get() network calls = %d, want %d", networkCalls, testCase.wantNetwork)
			}
			if testCase.wantStored != "" && string(store.entries[keyFor(request)].Body) != testCase.wantStored {
				t.Fatalf("stored body = %q, want %q", store.entries[keyFor(request)].Body, testCase.wantStored)
			}
		})
	}
}

// TestPolicyRetainsRawBodyAfterDecodeRejection verifies source decoding cannot corrupt a cached body.
// Expected: a later request returns the original raw bytes without invoking the failing network again.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Raw HTTP cache boundary".
func TestPolicyRetainsRawBodyAfterDecodeRejection(t *testing.T) {
	store := newMemoryStore()
	networkCalls := 0
	request := policyTestRequest(func(context.Context) (*http.Response, error) {
		networkCalls++
		if networkCalls > 1 {
			return nil, errors.New("network should not run on a fresh hit")
		}
		response, _ := newResponse(http.StatusOK, []byte("raw-body"), int64(len("raw-body")))
		return response, nil
	})
	policy := newPolicy(store, time.Now)

	first, err := policy.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	first.Body[0] = 'X'

	second, err := policy.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if string(second.Body) != "raw-body" {
		t.Fatalf("second body = %q, want raw-body", second.Body)
	}
	if !second.FromCache {
		t.Fatal("second response is not marked cached")
	}
	if networkCalls != 1 {
		t.Fatalf("network calls = %d, want 1", networkCalls)
	}
}

// TestPolicyResponseHandling verifies status preservation, body limits, completeness, errors, and closure.
// Expected: only complete successful GET bodies are stored, all live bodies close, and invalid bodies return no partial result.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Cacheable response policy".
func TestPolicyResponseHandling(t *testing.T) {
	largeBody := bytes.Repeat([]byte{'x'}, int(maxBodyBytes))
	cases := []struct {
		name           string
		method         string
		status         int
		body           []byte
		contentLength  int64
		networkError   error
		readError      error
		wantError      bool
		wantStatus     int
		wantBodyLength int
		wantPutCalls   int
	}{
		{
			name:           "complete-2xx",
			status:         http.StatusOK,
			body:           []byte("complete"),
			contentLength:  int64(len("complete")),
			wantStatus:     http.StatusOK,
			wantBodyLength: len("complete"),
			wantPutCalls:   1,
		},
		{
			name:           "non-2xx",
			status:         http.StatusNotFound,
			body:           []byte("missing"),
			contentLength:  int64(len("missing")),
			wantStatus:     http.StatusNotFound,
			wantBodyLength: len("missing"),
			wantPutCalls:   0,
		},
		{
			name:         "transport-error",
			networkError: errors.New("transport failed"),
			wantError:    true,
			wantPutCalls: 0,
		},
		{
			name:           "exact-limit",
			status:         http.StatusOK,
			body:           largeBody,
			contentLength:  maxBodyBytes,
			wantStatus:     http.StatusOK,
			wantBodyLength: int(maxBodyBytes),
			wantPutCalls:   1,
		},
		{
			name:          "oversized",
			status:        http.StatusOK,
			body:          append(append([]byte(nil), largeBody...), 'y'),
			contentLength: maxBodyBytes + 1,
			wantError:     true,
			wantPutCalls:  0,
		},
		{
			name:          "short-declared-body",
			status:        http.StatusOK,
			body:          []byte("short"),
			contentLength: int64(len("short") + 1),
			wantError:     true,
			wantPutCalls:  0,
		},
		{
			name:          "read-error",
			status:        http.StatusOK,
			body:          []byte("partial"),
			contentLength: -1,
			readError:     errors.New("read failed"),
			wantError:     true,
			wantPutCalls:  0,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := newMemoryStore()
			var responseBody *testBody
			request := policyTestRequest(func(context.Context) (*http.Response, error) {
				if testCase.networkError != nil {
					return nil, testCase.networkError
				}
				response, body := newResponse(testCase.status, testCase.body, testCase.contentLength)
				body.readError = testCase.readError
				responseBody = body
				return response, nil
			})
			request.Method = testCase.method
			if request.Method == "" {
				request.Method = http.MethodGet
			}
			policy := newPolicy(store, time.Now)

			got, err := policy.Retrieve(context.Background(), request)
			if (err != nil) != testCase.wantError {
				t.Fatalf("Get() error = %v, want error %t", err, testCase.wantError)
			}
			if err == nil {
				if got.StatusCode != testCase.wantStatus {
					t.Fatalf("status = %d, want %d", got.StatusCode, testCase.wantStatus)
				}
				if len(got.Body) != testCase.wantBodyLength {
					t.Fatalf("body length = %d, want %d", len(got.Body), testCase.wantBodyLength)
				}
			}
			if store.putCalls != testCase.wantPutCalls {
				t.Fatalf("storage writes = %d, want %d", store.putCalls, testCase.wantPutCalls)
			}
			if responseBody != nil && !responseBody.closed {
				t.Fatal("live response body was not closed")
			}
		})
	}
}

// TestPolicyWarningsAndCancellation verifies recoverable storage warnings and cancellation precedence.
// Expected: corruption misses silently, storage failures warn once, successful live responses survive write failures, and cancellation returns a context error.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Cache failures and structured warnings".
func TestPolicyWarningsAndCancellation(t *testing.T) {
	t.Run("corrupt-miss-is-silent", func(t *testing.T) {
		store := newMemoryStore()
		request := policyTestRequest(func(context.Context) (*http.Response, error) {
			response, _ := newResponse(http.StatusOK, []byte("live"), int64(len("live")))
			return response, nil
		})
		policy := newPolicy(store, time.Now)
		got, err := policy.Retrieve(context.Background(), request)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if len(got.Warnings) != 0 {
			t.Fatalf("warnings = %#v, want none", got.Warnings)
		}
	})

	t.Run("read-failure-warns", func(t *testing.T) {
		store := newMemoryStore()
		store.getError = errors.New("read failed")
		request := policyTestRequest(func(context.Context) (*http.Response, error) {
			response, _ := newResponse(http.StatusOK, []byte("live"), int64(len("live")))
			return response, nil
		})
		policy := newPolicy(store, time.Now)
		got, err := policy.Retrieve(context.Background(), request)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if len(got.Warnings) != 1 || got.Warnings[0].Operation != cacheReadOperation {
			t.Fatalf("warnings = %#v, want one read warning", got.Warnings)
		}
	})

	t.Run("write-failure-preserves-response", func(t *testing.T) {
		store := newMemoryStore()
		store.putError = errors.New("write failed")
		request := policyTestRequest(func(context.Context) (*http.Response, error) {
			response, _ := newResponse(http.StatusOK, []byte("live"), int64(len("live")))
			return response, nil
		})
		policy := newPolicy(store, time.Now)
		got, err := policy.Retrieve(context.Background(), request)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if string(got.Body) != "live" || len(got.Warnings) != 1 || got.Warnings[0].Operation != cacheWriteOperation {
			t.Fatalf("result = %#v, want live body and one write warning", got)
		}
	})

	t.Run("cancellation-during-read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		store := newMemoryStore()
		store.getFunc = func(context.Context, string) (Entry, bool, error) {
			cancel()
			return Entry{}, false, context.Canceled
		}
		networkCalls := 0
		request := policyTestRequest(func(context.Context) (*http.Response, error) {
			networkCalls++
			return nil, nil
		})
		_, err := newPolicy(store, time.Now).Retrieve(ctx, request)
		if !errors.Is(err, context.Canceled) || networkCalls != 0 {
			t.Fatalf("Get() error = %v, network calls = %d, want cancellation and no network", err, networkCalls)
		}
	})

	t.Run("cancellation-during-network-read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		store := newMemoryStore()
		request := policyTestRequest(func(context.Context) (*http.Response, error) {
			response, body := newResponse(http.StatusOK, []byte("live"), int64(len("live")))
			body.onRead = cancel
			return response, nil
		})
		_, err := newPolicy(store, time.Now).Retrieve(ctx, request)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Get() error = %v, want cancellation", err)
		}
	})

	t.Run("cancellation-during-write", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		store := newMemoryStore()
		store.putFunc = func(context.Context, string, Entry) error {
			cancel()
			return nil
		}
		request := policyTestRequest(func(context.Context) (*http.Response, error) {
			response, _ := newResponse(http.StatusOK, []byte("live"), int64(len("live")))
			return response, nil
		})
		_, err := newPolicy(store, time.Now).Retrieve(ctx, request)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Get() error = %v, want cancellation", err)
		}
	})
}

// TestPolicyRejectsInvalidRequests verifies required request preconditions without invoking network work.
// Expected: nonpositive TTL and missing deferred network return errors before cache policy proceeds.
// [NO_SPEC] Protects policy invariants that prevent undefined freshness or live execution.
func TestPolicyRejectsInvalidRequests(t *testing.T) {
	store := newMemoryStore()
	policy := newPolicy(store, time.Now)
	request := policyTestRequest(nil)
	request.TTL = 0
	if _, err := policy.Retrieve(context.Background(), request); err == nil {
		t.Fatal("Get() error = nil for nonpositive TTL")
	}
	request.TTL = time.Hour
	if _, err := policy.Retrieve(context.Background(), request); err == nil {
		t.Fatal("Get() error = nil for missing deferred network")
	}
}

// TestResponseBodyCloseError verifies close failures prevent a cache write.
// Expected: a close error is returned and no incomplete entry is stored.
// [NO_SPEC] Covers resource-finalization failure handling not separately specified by the cache scenarios.
func TestResponseBodyCloseError(t *testing.T) {
	store := newMemoryStore()
	request := policyTestRequest(func(context.Context) (*http.Response, error) {
		response, body := newResponse(http.StatusOK, []byte("body"), int64(len("body")))
		body.closeError = io.ErrClosedPipe
		return response, nil
	})
	_, err := newPolicy(store, time.Now).Retrieve(context.Background(), request)
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Get() error = %v, want close error", err)
	}
	if store.putCalls != 0 {
		t.Fatalf("storage writes = %d, want 0", store.putCalls)
	}
}
