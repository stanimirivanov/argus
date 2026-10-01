package contracts

import (
	"fmt"

	contractassets "github.com/stanimirivanov/argus/contracts"
)

// mustReadSchema resolves a compile-time-known schema path. A missing embedded
// artifact is a broken build, not malformed caller input, so fail at startup.
func mustReadSchema(name string) []byte {
	data, err := contractassets.ReadGeneratedSchema(name)
	if err != nil {
		panic(fmt.Sprintf("read embedded contract schema %q: %v", name, err))
	}

	return data
}
