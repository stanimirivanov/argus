// Package catalog owns the repository catalog's domain vocabulary, invariants,
// use cases, and narrow persistence port. Transport adapters convert external
// documents into these values, while storage adapters implement the port
// without exposing driver types to catalog policy.
package catalog
