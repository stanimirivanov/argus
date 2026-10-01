package contracts

import "path/filepath"

func fixturePath(name string) string {
	return filepath.Join("..", "..", "contracts", filepath.FromSlash(name))
}
