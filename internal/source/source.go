// Package source defines source adapters, metric metadata, and request construction.
package source

import (
	"context"
	"net/http"

	"github.com/kafumanto/ai-pareto/internal/cache"
	"github.com/kafumanto/ai-pareto/internal/domain"
)

// MetricInfo describes one source-native metric available to objective configuration.
type MetricInfo struct {
	// Name is the source-native metric identifier.
	Name string
	// Unit describes the metric's measurement unit.
	Unit string
	// Description explains the metric and any source-specific task definition.
	Description string
	// SuggestedGoal is the canonical initial goal, such as min or max.
	SuggestedGoal string
}

// HTTPDoer executes an HTTP request supplied by a source adapter.
//
// A normal *http.Client satisfies HTTPDoer. Adapters receive this dependency
// during construction and must not create an application HTTP client.
type HTTPDoer interface {
	// Do executes request and returns its response or a transport error.
	Do(request *http.Request) (*http.Response, error)
}

// SourceRequest contains request-scoped services and optional credentials for a source.
//
// The value is supplied to a source constructor for one application request.
// The registry does not retain the value or any dependency stored in it.
type SourceRequest struct {
	// HTTP executes remote requests for the source; nil is allowed for non-remote sources.
	HTTP HTTPDoer
	// Cache performs shared cache-aware GETs for the source. The retriever owns key derivation,
	// freshness, refresh behavior, and Store access; nil is allowed for non-remote sources.
	Cache cache.Retriever
	// Credential contains request-scoped credential bytes and is optional.
	Credential []byte
}

// Source retrieves records and publishes source-native metric metadata.
type Source interface {
	// Name returns the stable source identifier.
	Name() string
	// Attribution returns static attribution for the source adapter.
	Attribution() string
	// Fetch retrieves a complete dataset or returns an error.
	Fetch(ctx context.Context) ([]domain.Record, error)
	// Metrics returns ordered metric metadata, including fields discovered by Fetch when supported.
	Metrics() []MetricInfo
}
