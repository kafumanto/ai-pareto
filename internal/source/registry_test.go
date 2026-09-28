package source

import (
	"context"
	"reflect"
	"testing"

	"github.com/kafumanto/ai-pareto/internal/domain"
)

// fakeSource records constructor input for registry contract tests.
// Expected: each instance retains only the credential supplied to its constructor.
// [NO_SPEC] Supports request-scoped registry verification without a production source.
type fakeSource struct {
	// credential stores the request credential copied by the fake constructor.
	credential []byte
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
 * Expected: each request receives only its own credential, receives a distinct source, and leaves registry metadata credential-free.
 * [SPEC] openspec/changes/establish-portable-domain-contracts/specs/portable-domain-contracts/spec.md, heading "Requirement: Source construction and retrieval contract".
 */
func TestRegistryConstructsRequestScopedSources(t *testing.T) {
	registry := NewSourceRegistry()
	err := registry.Register(SourceRegistration{
		Name:        "fake",
		Attribution: "test",
		Constructor: func(request SourceRequest) Source {
			return &fakeSource{credential: append([]byte(nil), request.Credential...)}
		},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	first, err := registry.NewSource("fake", SourceRequest{Credential: []byte("first")})
	if err != nil {
		t.Fatalf("NewSource(first) error = %v", err)
	}
	second, err := registry.NewSource("fake", SourceRequest{Credential: []byte("second")})
	if err != nil {
		t.Fatalf("NewSource(second) error = %v", err)
	}
	if first == second {
		t.Fatal("NewSource() returned the same source instance twice")
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
