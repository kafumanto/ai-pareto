package source

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/kafumanto/ai-pareto/internal/cache"
	"github.com/kafumanto/ai-pareto/internal/domain"
)

// fakeSource records constructor input for registry contract tests.
// Expected: each instance retains only the dependencies supplied to its constructor.
// [NO_SPEC] Supports request-scoped registry verification without a production source.
type fakeSource struct {
	// httpDoer stores the request-scoped HTTP dependency received by the fake constructor.
	httpDoer HTTPDoer
	// retriever stores the request-scoped cache dependency received by the fake constructor.
	retriever cache.Retriever
	// credential stores the request credential copied by the fake constructor.
	credential []byte
}

// fakeHTTPDoer identifies one request-scoped HTTP dependency in registry tests.
// [NO_SPEC] Supports dependency-isolation assertions without performing network I/O.
type fakeHTTPDoer struct {
	// name distinguishes otherwise equivalent HTTP test dependencies.
	name string
}

// Do returns no response because registry tests only compare dependency identity.
// [NO_SPEC] Implements HTTPDoer for request-scoped dependency tests.
func (fakeHTTPDoer) Do(*http.Request) (*http.Response, error) {
	return nil, nil
}

// fakeCacheRetriever identifies one request-scoped cache dependency in registry tests.
// [NO_SPEC] Supports cache dependency-isolation assertions without storing entries.
type fakeCacheRetriever struct {
	// name distinguishes otherwise equivalent cache test dependencies.
	name string
}

// Retrieve returns an empty result because registry tests only compare dependency identity.
// [NO_SPEC] Implements cache.Retriever for request-scoped dependency tests.
func (fakeCacheRetriever) Retrieve(context.Context, cache.Request) (cache.Result, error) {
	return cache.Result{}, nil
}

// Name returns the test source identifier.
// [NO_SPEC] Supports the fake Source implementation used by registry tests.
func (fakeSource) Name() string {
	return "fake"
}

// Attribution returns static test attribution.
// [NO_SPEC] Supports the fake Source implementation used by registry tests.
func (fakeSource) Attribution() string {
	return "test"
}

// Fetch returns no records because registry tests do not retrieve data.
// [NO_SPEC] Supports the fake Source implementation used by registry tests.
func (fakeSource) Fetch(context.Context) ([]domain.Record, error) {
	return nil, nil
}

// Metrics returns no metrics because registry tests do not retrieve data.
// [NO_SPEC] Supports the fake Source implementation used by registry tests.
func (fakeSource) Metrics() []MetricInfo {
	return nil
}

/**
 * TestRegistryConstructsRequestScopedSources verifies separate construction and static registry state.
 * Expected: each request receives only its own HTTP, cache, and credential dependencies, receives a distinct source, and leaves registry metadata request-free.
 * [SPEC] openspec/changes/implement-raw-http-cache/specs/portable-domain-contracts/spec.md, heading "Requirement: Source construction and retrieval contract".
 */
func TestRegistryConstructsRequestScopedSources(t *testing.T) {
	registry := NewSourceRegistry()
	err := registry.Register(SourceRegistration{
		Name:        "fake",
		Attribution: "test",
		Constructor: func(request SourceRequest) Source {
			return &fakeSource{
				httpDoer:   request.HTTP,
				retriever:  request.Cache,
				credential: append([]byte(nil), request.Credential...),
			}
		},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	firstHTTP := &fakeHTTPDoer{name: "first"}
	firstCache := &fakeCacheRetriever{name: "first"}
	first, err := registry.NewSource("fake", SourceRequest{
		HTTP:       firstHTTP,
		Cache:      firstCache,
		Credential: []byte("first"),
	})
	if err != nil {
		t.Fatalf("NewSource(first) error = %v", err)
	}
	secondHTTP := &fakeHTTPDoer{name: "second"}
	secondCache := &fakeCacheRetriever{name: "second"}
	second, err := registry.NewSource("fake", SourceRequest{
		HTTP:       secondHTTP,
		Cache:      secondCache,
		Credential: []byte("second"),
	})
	if err != nil {
		t.Fatalf("NewSource(second) error = %v", err)
	}
	if first == second {
		t.Fatal("NewSource() returned the same source instance twice")
	}

	if got := first.(*fakeSource).httpDoer; got != firstHTTP {
		t.Fatalf("first HTTP dependency = %p, want %p", got, firstHTTP)
	}
	if got := first.(*fakeSource).retriever; got != firstCache {
		t.Fatalf("first cache dependency = %p, want %p", got, firstCache)
	}
	if got := second.(*fakeSource).httpDoer; got != secondHTTP {
		t.Fatalf("second HTTP dependency = %p, want %p", got, secondHTTP)
	}
	if got := second.(*fakeSource).retriever; got != secondCache {
		t.Fatalf("second cache dependency = %p, want %p", got, secondCache)
	}
	if got := first.(*fakeSource).credential; !reflect.DeepEqual(got, []byte("first")) {
		t.Fatalf("first credential = %q, want first", got)
	}
	if got := second.(*fakeSource).credential; !reflect.DeepEqual(got, []byte("second")) {
		t.Fatalf("second credential = %q, want second", got)
	}

	registrations := registry.Registrations()
	if len(registrations) != 1 || registrations[0].Name != "fake" || registrations[0].Attribution != "test" {
		t.Fatalf("Registrations() = %#v, want static fake registration", registrations)
	}
}

/**
 * TestRegistryRejectsInvalidRegistrations protects constructor lookup invariants.
 * Expected: empty names, nil constructors, and duplicate names return errors.
 * [NO_SPEC] Protects registry state integrity required by source construction without a separate scenario.
 */
func TestRegistryRejectsInvalidRegistrations(t *testing.T) {
	registry := NewSourceRegistry()
	if err := registry.Register(SourceRegistration{}); err == nil {
		t.Fatal("Register() error = nil for empty registration")
	}
	if err := registry.Register(SourceRegistration{Name: "fake"}); err == nil {
		t.Fatal("Register() error = nil for nil constructor")
	}
	registration := SourceRegistration{Name: "fake", Constructor: func(SourceRequest) Source { return fakeSource{} }}
	if err := registry.Register(registration); err != nil {
		t.Fatalf("Register(valid) error = %v", err)
	}
	if err := registry.Register(registration); err == nil {
		t.Fatal("Register() error = nil for duplicate name")
	}
}
