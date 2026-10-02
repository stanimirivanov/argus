package githubreview

import (
	"reflect"
	"testing"
)

func TestConcreteReviewRolesExposeOnlyTheirAuthority(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		role   any
		method string
	}{
		{name: "source reader", role: (*SourceReader)(nil), method: "LoadSource"},
		{name: "publisher", role: (*Publisher)(nil), method: "Publish"},
		{name: "outcome observer", role: (*OutcomeObserver)(nil), method: "ObserveOutcome"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			typeOfRole := reflect.TypeOf(test.role)
			if typeOfRole.NumMethod() != 1 || typeOfRole.Method(0).Name != test.method {
				t.Fatalf("%s exposes methods beyond %s: %v", test.name, test.method, typeOfRole)
			}
		})
	}
}
