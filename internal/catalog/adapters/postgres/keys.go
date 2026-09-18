package postgres

import (
	"cmp"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func compareRepositoryIdentities(left, right catalog.RepositoryIdentity) int {
	if compared := cmp.Compare(left.Provider, right.Provider); compared != 0 {
		return compared
	}
	if compared := cmp.Compare(left.Host, right.Host); compared != 0 {
		return compared
	}

	return cmp.Compare(left.ProviderRepositoryID, right.ProviderRepositoryID)
}
