package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kafumanto/ai-pareto/internal/domain"
)

// TestDefault verifies the exact shared duration defaults and the absence of implicit selections.
// Expected: Default returns a 15-second timeout, a 24-hour cache TTL, an empty source name, and no objectives.
// [SPEC] openspec/changes/establish-normalized-configuration-contracts/specs/normalized-configuration/spec.md, heading "Requirement: Shared duration defaults".
func TestDefault(t *testing.T) {
	// Construct the shared default value once so every expected field is compared exactly.
	got := Default()
	want := Configuration{
		Source: Source{Timeout: 15 * time.Second},
		Cache:  Cache{TTL: 24 * time.Hour},
	}

	// Confirm defaults do not select a source or invent optimization objectives.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Default() = %#v, want %#v", got, want)
	}
	// Require no implicit source selection in the shared defaults.
	if got.Source.Name != "" {
		t.Fatalf("Default().Source.Name = %q, want empty", got.Source.Name)
	}
	// Require no implicit optimization objective in the shared defaults.
	if got.Optimization.Objectives != nil {
		t.Fatalf("Default().Optimization.Objectives = %#v, want nil", got.Optimization.Objectives)
	}
}

// TestValidateSourceAndDurations covers source selection, positive durations, error identity, and immutability.
// Expected: zero and negative values fail with field context, while a complete configuration succeeds unchanged.
// [SPEC] openspec/changes/establish-normalized-configuration-contracts/specs/normalized-configuration/spec.md, headings "Requirement: Positive shared durations", "Requirement: Required source selection", and "Requirement: Authoritative non-mutating validation".
func TestValidateSourceAndDurations(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Configuration)
		wantField string
	}{
		{
			name: "valid",
			mutate: func(configuration *Configuration) {
				// Keep the complete test configuration unchanged for the valid case.
			},
		},
		{
			name: "empty source name",
			mutate: func(configuration *Configuration) {
				configuration.Source.Name = ""
			},
			wantField: "source.name",
		},
		{
			name: "zero source timeout",
			mutate: func(configuration *Configuration) {
				configuration.Source.Timeout = 0
			},
			wantField: "source.timeout",
		},
		{
			name: "negative source timeout",
			mutate: func(configuration *Configuration) {
				configuration.Source.Timeout = -time.Second
			},
			wantField: "source.timeout",
		},
		{
			name: "zero cache TTL",
			mutate: func(configuration *Configuration) {
				configuration.Cache.TTL = 0
			},
			wantField: "cache.ttl",
		},
		{
			name: "negative cache TTL",
			mutate: func(configuration *Configuration) {
				configuration.Cache.TTL = -time.Hour
			},
			wantField: "cache.ttl",
		},
	}

	// Run every field case against a fresh complete configuration so failures remain independent.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := validConfiguration()
			test.mutate(&configuration)
			original := cloneConfiguration(configuration)

			// Validate the effective values without allowing validation to apply defaults.
			err := Validate(configuration)
			if test.wantField == "" {
				// Require the complete configuration to pass semantic validation.
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
			} else {
				// Require each invalid case to expose its field and the shared error identity.
				assertValidationError(t, err, test.wantField)
			}

			// Compare against a copied objective slice so mutation of the input is observable.
			if !reflect.DeepEqual(configuration, original) {
				t.Fatalf("Validate() mutated configuration: got %#v, want %#v", configuration, original)
			}
		})
	}
}

// TestValidateObjectives covers canonical metric contracts, ordered objectives, and deferred cardinality rules.
// Expected: complete one-, two-, and three-objective configurations pass, while incomplete or non-canonical entries fail.
// [SPEC] openspec/changes/establish-normalized-configuration-contracts/specs/normalized-configuration/spec.md, heading "Requirement: Canonical initial objectives".
func TestValidateObjectives(t *testing.T) {
	ordered := []domain.Objective{
		domain.NewObjective(domain.NewMetricExpression("cost"), domain.NewMinGoal()),
		domain.NewObjective(domain.NewMetricExpression("quality"), domain.NewMaxGoal()),
	}
	tests := []struct {
		name       string
		objectives []domain.Objective
		wantField  string
	}{
		{
			name:       "one objective",
			objectives: ordered[:1],
		},
		{
			name:       "ordered objectives",
			objectives: ordered,
		},
		{
			name: "three objectives",
			objectives: append(append([]domain.Objective{}, ordered...),
				domain.NewObjective(domain.NewMetricExpression("latency"), domain.NewMinGoal())),
		},
		{
			name: "duplicate expressions",
			objectives: []domain.Objective{
				domain.NewObjective(domain.NewMetricExpression("cost"), domain.NewMinGoal()),
				domain.NewObjective(domain.NewMetricExpression("cost"), domain.NewMaxGoal()),
			},
		},
		{
			name:      "empty collection",
			wantField: "optimization.objectives",
		},
		{
			name: "nil value expression",
			objectives: []domain.Objective{
				domain.NewObjective(nil, domain.NewMinGoal()),
			},
			wantField: "optimization.objectives[0].value",
		},
		{
			name: "nil goal",
			objectives: []domain.Objective{
				domain.NewObjective(domain.NewMetricExpression("cost"), nil),
			},
			wantField: "optimization.objectives[0].goal",
		},
		{
			name: "empty metric name",
			objectives: []domain.Objective{
				domain.NewObjective(domain.NewMetricExpression(""), domain.NewMinGoal()),
			},
			wantField: "optimization.objectives[0].value.required_metrics",
		},
		{
			name: "multiple required metrics",
			objectives: []domain.Objective{
				domain.NewObjective(testValueExpression{
					expression: "cost",
					metrics:    []string{"cost", "quality"},
				}, domain.NewMinGoal()),
			},
			wantField: "optimization.objectives[0].value.required_metrics",
		},
		{
			name: "mismatched metric metadata",
			objectives: []domain.Objective{
				domain.NewObjective(testValueExpression{
					expression: "canonical-cost",
					metrics:    []string{"source-cost"},
				}, domain.NewMinGoal()),
			},
			wantField: "optimization.objectives[0].value.expression",
		},
		{
			name: "unsupported goal",
			objectives: []domain.Objective{
				domain.NewObjective(domain.NewMetricExpression("cost"), testGoal{expression: "average"}),
			},
			wantField: "optimization.objectives[0].goal.expression",
		},
	}

	// Run every objective shape against a fresh source and duration configuration.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := validConfiguration()
			configuration.Optimization.Objectives = test.objectives
			original := cloneConfiguration(configuration)

			// Validate objective semantics without enforcing later count or duplicate rules.
			err := Validate(configuration)
			if test.wantField == "" {
				// Require every complete objective collection to pass unchanged.
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
			} else {
				// Require every incomplete or non-canonical objective to identify its field.
				assertValidationError(t, err, test.wantField)
			}

			// Confirm validation preserves declaration order and all objective values.
			if !reflect.DeepEqual(configuration, original) {
				t.Fatalf("Validate() mutated objectives: got %#v, want %#v", configuration, original)
			}
		})
	}

	// Confirm the accepted ordered pair retains the canonical expressions in declaration order.
	configuration := validConfiguration()
	configuration.Optimization.Objectives = ordered
	// Require the complete ordered pair to pass validation before inspecting its expressions.
	if err := Validate(configuration); err != nil {
		t.Fatalf("Validate(ordered objectives) error = %v", err)
	}
	// Require the first objective to retain its declared canonical expression.
	if got := configuration.Optimization.Objectives[0].Value.Expression(); got != "cost" {
		t.Fatalf("first objective expression = %q, want %q", got, "cost")
	}
	// Require the second objective to retain its declared canonical expression.
	if got := configuration.Optimization.Objectives[1].Value.Expression(); got != "quality" {
		t.Fatalf("second objective expression = %q, want %q", got, "quality")
	}
}

// TestConfigurationBoundaryContainsOnlyNormalizedValues verifies the concrete configuration graph.
// Expected: fields contain only source, cache, optimization, string, duration, slice, and domain-objective values.
// [SPEC] openspec/changes/establish-normalized-configuration-contracts/specs/normalized-configuration/spec.md, headings "Requirement: Runtime-neutral credential-free boundary" and "Requirement: Deferred ordered catalog boundary".
func TestConfigurationBoundaryContainsOnlyNormalizedValues(t *testing.T) {
	// Check every configuration layer for its exact field names and normalized types.
	assertStructFields(t, "Configuration", reflect.TypeOf(Configuration{}), map[string]reflect.Type{
		"Source":       reflect.TypeOf(Source{}),
		"Cache":        reflect.TypeOf(Cache{}),
		"Optimization": reflect.TypeOf(Optimization{}),
	})
	assertStructFields(t, "Source", reflect.TypeOf(Source{}), map[string]reflect.Type{
		"Name":    reflect.TypeOf(""),
		"Timeout": reflect.TypeOf(time.Duration(0)),
	})
	assertStructFields(t, "Cache", reflect.TypeOf(Cache{}), map[string]reflect.Type{
		"TTL": reflect.TypeOf(time.Duration(0)),
	})
	assertStructFields(t, "Optimization", reflect.TypeOf(Optimization{}), map[string]reflect.Type{
		"Objectives": reflect.TypeOf([]domain.Objective{}),
	})
}

// testValueExpression supplies controlled expression metadata for validation tests.
type testValueExpression struct {
	// expression is the canonical expression reported by the test value.
	expression string
	// metrics contains the source-native metric names reported by the test value.
	metrics []string
}

// Expression returns the configured canonical expression for the test value.
func (expression testValueExpression) Expression() string {
	return expression.expression
}

// RequiredMetrics returns a copy of the configured source-native metric names.
func (expression testValueExpression) RequiredMetrics() []string {
	// Copy the metadata slice so validation cannot mutate test-owned objective metadata.
	return append([]string(nil), expression.metrics...)
}

// Evaluate returns a finite placeholder value because metadata validation does not evaluate records.
func (testValueExpression) Evaluate(_ domain.Record) (float64, error) {
	return 0, nil
}

// testGoal supplies a controlled goal expression for validation tests.
type testGoal struct {
	// expression is the canonical goal expression reported by the test goal.
	expression string
}

// Expression returns the configured canonical goal expression for the test goal.
func (goal testGoal) Expression() string {
	return goal.expression
}

// Loss returns the input value unchanged because validation does not evaluate objectives.
func (testGoal) Loss(value float64) (float64, error) {
	return value, nil
}

// validConfiguration returns a complete normalized configuration for validation tests.
// The returned value includes two ordered metric objectives and fresh objective storage.
func validConfiguration() Configuration {
	return Configuration{
		Source: Source{Name: "test-source", Timeout: time.Second},
		Cache:  Cache{TTL: time.Hour},
		Optimization: Optimization{Objectives: []domain.Objective{
			domain.NewObjective(domain.NewMetricExpression("cost"), domain.NewMinGoal()),
			domain.NewObjective(domain.NewMetricExpression("quality"), domain.NewMaxGoal()),
		}},
	}
}

// cloneConfiguration copies configuration and its objective slice for mutation checks.
// The input configuration remains unchanged, while nested objective contracts retain their values.
func cloneConfiguration(configuration Configuration) Configuration {
	// Copy the objective slice so validation mutations would differ from the preserved input.
	configuration.Optimization.Objectives = append([]domain.Objective(nil), configuration.Optimization.Objectives...)
	return configuration
}

// assertValidationError verifies ErrInvalid identity and field context for a validation failure.
// The test fails when err is nil, lacks the sentinel, or omits the expected field path.
func assertValidationError(t *testing.T, err error, field string) {
	t.Helper()
	// Reject a missing error before checking its identity or text.
	if err == nil {
		t.Fatalf("Validate() error = nil, want field %q", field)
	}
	// Require the shared sentinel so callers can classify every semantic failure.
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Validate() error = %v, want errors.Is(error, ErrInvalid)", err)
	}
	// Require the structural field path so the caller can correct the invalid value.
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("Validate() error = %v, want field context %q", err, field)
	}
}

// assertStructFields verifies that a struct has exactly the expected named field types.
// The helper fails when the concrete configuration graph exposes an undeclared runtime value.
func assertStructFields(t *testing.T, name string, actual reflect.Type, expected map[string]reflect.Type) {
	t.Helper()
	// Require a struct so field inspection reflects the configuration contract.
	if actual.Kind() != reflect.Struct {
		t.Fatalf("%s kind = %s, want struct", name, actual.Kind())
	}
	// Require no extra or missing fields before comparing individual field types.
	if actual.NumField() != len(expected) {
		t.Fatalf("%s field count = %d, want %d", name, actual.NumField(), len(expected))
	}
	// Compare every declared field type against the normalized contract.
	for fieldName, wantType := range expected {
		field, found := actual.FieldByName(fieldName)
		// Reject a missing field before comparing its type.
		if !found {
			t.Fatalf("%s is missing field %q", name, fieldName)
		}
		// Reject any field type that carries a non-normalized value.
		if field.Type != wantType {
			t.Fatalf("%s.%s type = %s, want %s", name, fieldName, field.Type, wantType)
		}
	}
}
