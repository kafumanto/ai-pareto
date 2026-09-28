package source

import (
	"errors"
	"fmt"
)

// SourceConstructor builds one source for one request's dependencies.
type SourceConstructor func(request SourceRequest) Source

// SourceRegistration contains the non-secret static metadata and constructor for one source.
type SourceRegistration struct {
	// Name is the stable source identifier used for lookup.
	Name string
	// Attribution is static source attribution exposed without fetching.
	Attribution string
	// Constructor builds a fresh source for the current request.
	Constructor SourceConstructor
}

// SourceRegistry stores source registrations without retaining request-scoped state.
//
// Registry entries contain only constructors and static metadata. A registry
// does not retain credentials, HTTP clients, metric catalogs, or source values.
type SourceRegistry struct {
	// entries maps source names to static registrations and constructors.
	entries map[string]SourceRegistration
	// order preserves deterministic metadata order without retaining source instances.
	order []string
}

// NewSourceRegistry returns an empty source registry.
func NewSourceRegistry() *SourceRegistry {
	return &SourceRegistry{entries: make(map[string]SourceRegistration)}
}

// Register adds one source registration to the registry.
//
// Register returns an error for an empty name, a nil constructor, or a duplicate
// name. Registration stores no request-scoped dependency.
func (registry *SourceRegistry) Register(registration SourceRegistration) error {
	if registration.Name == "" {
		return errors.New("source registration name is empty")
	}
	if registration.Constructor == nil {
		return fmt.Errorf("source %q has no constructor", registration.Name)
	}
	if _, exists := registry.entries[registration.Name]; exists {
		return fmt.Errorf("source %q is already registered", registration.Name)
	}
	if registry.entries == nil {
		registry.entries = make(map[string]SourceRegistration)
	}

	// Store only static registration data and retain insertion order for stable metadata reads.
	registry.entries[registration.Name] = registration
	registry.order = append(registry.order, registration.Name)
	return nil
}

// NewSource constructs a fresh source for name and request-scoped dependencies.
func (registry *SourceRegistry) NewSource(name string, request SourceRequest) (Source, error) {
	registration, exists := registry.entries[name]
	if !exists {
		return nil, fmt.Errorf("source %q is not registered", name)
	}

	// Invoke the stored constructor only for this request and retain no constructed source.
	return registration.Constructor(request), nil
}

// Registrations returns a copy of registrations in registration order.
func (registry *SourceRegistry) Registrations() []SourceRegistration {
	registrations := make([]SourceRegistration, 0, len(registry.order))
	for _, name := range registry.order {
		registrations = append(registrations, registry.entries[name])
	}
	return registrations
}
