// Package domain defines source-independent records and optimization contracts.
package domain

// Record is one comparable variant returned by a source adapter.
//
// Source adapters must assign an ID that is unique within one fetched dataset.
// Adapters must include only finite decoded values in Metrics and must preserve
// variant identity in ID and Attributes, including for derived variants.
type Record struct {
	// ID identifies the comparable variant within one fetched source dataset.
	ID string
	// Name is the display name for the variant.
	Name string
	// Source identifies the source adapter that produced the variant.
	Source string
	// Provider identifies the upstream provider of the variant data.
	Provider string
	// Metrics contains finite source-native numerical values keyed by metric name.
	Metrics map[string]float64
	// Attributes contains string metadata that does not participate in dominance.
	Attributes map[string]string
}
