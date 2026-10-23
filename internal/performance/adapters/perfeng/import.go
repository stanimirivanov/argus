package perfeng

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
)

// Import revalidates complete source pages at the persistence boundary and builds
// owned inward evidence. No credential, endpoint or execution authority is stored.
func Import(pages []json.RawMessage, expected Expectations, at time.Time) (catalogsync.Snapshot, error) {
	if err := Validate(pages, expected, at); err != nil {
		return catalogsync.Snapshot{}, err
	}
	var evidence catalogsync.Evidence
	evidence.Scope = catalogsync.Scope{Issuer: expected.Issuer, PrincipalIssuer: expected.PrincipalIssuer, PrincipalID: expected.PrincipalID}
	pin := sha256.Sum256(schemaBytes)
	evidence.SchemaSHA256 = hex.EncodeToString(pin[:])
	for index, data := range pages {
		var page struct {
			Snapshot struct {
				Reference               struct{ ID, Revision, SHA256 string }
				GeneratedAt, ValidUntil time.Time
			}
			Items []struct {
				Kind, State string
				Reference   struct{ Issuer, ID, Revision, SHA256 string }
			}
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return catalogsync.Snapshot{}, err
		}
		if index == 0 {
			evidence.ID, evidence.Revision, evidence.ContentSHA256 = page.Snapshot.Reference.ID, page.Snapshot.Reference.Revision, page.Snapshot.Reference.SHA256
			evidence.GeneratedAt, evidence.ValidUntil = page.Snapshot.GeneratedAt, page.Snapshot.ValidUntil
		}
		for _, item := range page.Items {
			evidence.Resources = append(evidence.Resources, catalogsync.Resource{Kind: item.Kind, Issuer: item.Reference.Issuer, ID: item.Reference.ID, Revision: item.Reference.Revision, SHA256: item.Reference.SHA256, State: item.State})
		}
	}
	data, err := json.Marshal(pages)
	if err != nil {
		return catalogsync.Snapshot{}, err
	}
	evidence.Pages = data

	return catalogsync.NewSnapshot(evidence)
}
