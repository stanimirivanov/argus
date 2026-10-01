package contracts_test

import "path/filepath"

// fixturePath keeps the shared compatibility corpus in the portable contract
// workspace even though the Go binding is product-internal.
func fixturePath(name string) string {
	return filepath.Join("..", "..", "contracts", filepath.FromSlash(name))
}
