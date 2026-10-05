// Package config defines runtime-neutral effective configuration for shared execution.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/kafumanto/ai-pareto/internal/domain"
)

const (
	// DefaultSourceTimeout is the shared 15-second timeout for one source request.
	DefaultSourceTimeout = 15 * time.Second
	// DefaultCacheTTL is the shared 24-hour freshness period for one cached response.
	DefaultCacheTTL = 24 * time.Hour
)

var (
	// ErrInvalid indicates that effective configuration violates a shared semantic contract.
	ErrInvalid = errors.New("invalid configuration")
)

// Configuration contains the complete runtime-neutral effective configuration.
//
// The value does not own credentials, native paths, output settings, refresh
// state, profile data, process input, environment values, or filesystem values.
type Configuration struct {
	// Source selects the source and its request timeout.
	Source Source
	// Cache defines the shared cache freshness period.
	Cache Cache
	// Optimization defines ordered objectives for Pareto analysis.
	Optimization Optimization
}

// Source contains the selected source name and shared request timeout.
type Source struct {
	// Name identifies the source adapter to use.
	Name string
	// Timeout is the maximum duration for one source request.
	Timeout time.Duration
}

// Cache contains shared cache freshness settings.
type Cache struct {
	// TTL is the freshness period for cached source data.
	TTL time.Duration
}

// Optimization contains ordered objectives for shared Pareto analysis.
type Optimization struct {
	// Objectives preserves objectives in the caller's declaration order.
	Objectives []domain.Objective
}

// Default returns configuration with the shared duration defaults only.
//
// Source selection and objectives remain unset until a caller supplies them.
func Default() Configuration {
	return Configuration{
		Source: Source{Timeout: DefaultSourceTimeout},
		Cache:  Cache{TTL: DefaultCacheTTL},
	}
}

// Validate checks intrinsic effective configuration semantics without mutating configuration.
//
// The function returns an error wrapping ErrInvalid and naming the first invalid
// field in structural order. It does not apply defaults or query runtime registries.
func Validate(configuration Configuration) error {
	// Require a caller-selected source before validating source request settings.
	if configuration.Source.Name == "" {
		return invalid("source.name", "must not be empty")
	}

	// Reject explicit zero and negative timeouts instead of treating them as omitted values.
	if configuration.Source.Timeout <= 0 {
		return invalid("source.timeout", "must be positive")
	}

	// Reject explicit zero and negative cache periods instead of applying the shared default.
	if configuration.Cache.TTL <= 0 {
		return invalid("cache.ttl", "must be positive")
	}

	// Require at least one objective before inspecting each ordered objective contract.
	if len(configuration.Optimization.Objectives) == 0 {
		return invalid("optimization.objectives", "must not be empty")
	}

	// Validate objectives in declaration order so the first failure is deterministic.
	for index, objective := range configuration.Optimization.Objectives {
		// Reject an objective without a value expression before inspecting metric metadata.
		if objective.Value == nil {
			return invalid(fmt.Sprintf("optimization.objectives[%d].value", index), "must not be nil")
		}
		// Reject an objective without a goal before inspecting its canonical expression.
		if objective.Goal == nil {
			return invalid(fmt.Sprintf("optimization.objectives[%d].goal", index), "must not be nil")
		}

		// Require one named metric whose canonical expression identifies the same metric.
		requiredMetrics := objective.Value.RequiredMetrics()
		// Reject missing, empty, or multiple required metrics at the initial boundary.
		if len(requiredMetrics) != 1 || requiredMetrics[0] == "" {
			return invalid(fmt.Sprintf("optimization.objectives[%d].value.required_metrics", index), "must contain one non-empty metric")
		}
		// Reject an expression whose canonical representation does not identify the required metric.
		if objective.Value.Expression() != requiredMetrics[0] {
			return invalid(fmt.Sprintf("optimization.objectives[%d].value.expression", index), "must match its required metric")
		}

		// Accept only the canonical initial minimum and maximum goal expressions.
		goalExpression := objective.Goal.Expression()
		// Reject later or unsupported goal expressions at the initial boundary.
		if goalExpression != "min" && goalExpression != "max" {
			return invalid(fmt.Sprintf("optimization.objectives[%d].goal.expression", index), "must be min or max")
		}
	}

	return nil
}

// invalid wraps a field-specific semantic failure with ErrInvalid.
func invalid(field string, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalid, field, reason)
}
