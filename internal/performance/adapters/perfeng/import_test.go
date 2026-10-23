package perfeng

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestImportOwnsCompleteCanonicalEvidence(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"pages", "empty"} {
		source := fixture(t, name)
		snapshot, err := Import(source, fixtureScope, fixtureTime)
		if err != nil {
			t.Fatal(err)
		}
		data := snapshot.Evidence()
		if data.Scope.PrincipalID != fixtureScope.PrincipalID || data.SchemaSHA256 != "becf420e60231bd0411352ba94b63c8450aa1abb0cf569c0e1634915eb19ce75" {
			t.Fatal("lost authority or schema pin")
		}
		want := 8
		if name == "empty" {
			want = 0
		}
		if len(data.Resources) != want || !snapshot.Fresh(fixtureTime) || snapshot.Fresh(data.ValidUntil) {
			t.Fatal("lost complete resource projection or freshness boundary")
		}
		indentPages(t, source)
		retry, err := Import(source, fixtureScope, fixtureTime)
		if err != nil || retry.Fingerprint() != snapshot.Fingerprint() {
			t.Fatalf("lexical retry changed immutable identity: %v", err)
		}
		data.Pages[0] = '!'
		if len(data.Resources) > 0 {
			data.Resources[0].ID = "changed"
		}
		if bytes.Equal(data.Pages, snapshot.Evidence().Pages) {
			t.Fatal("caller can mutate owned evidence")
		}
	}
}

func indentPages(t *testing.T, source []json.RawMessage) {
	t.Helper()
	for index, raw := range source {
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, raw, "", "  "); err != nil {
			t.Fatal(err)
		}
		source[index] = formatted.Bytes()
	}
}

func TestImportRejectsWrongAuthority(t *testing.T) {
	t.Parallel()
	wrong := fixtureScope
	wrong.PrincipalID = "another-principal"
	if _, err := Import(fixture(t, "pages"), wrong, fixtureTime); err == nil {
		t.Fatal("imported another principal's evidence")
	}
}
