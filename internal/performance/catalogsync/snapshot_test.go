package catalogsync

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func evidence() Evidence {
	return Evidence{
		Scope: Scope{"provider", "auth", "principal"}, ID: "catalog", Revision: "1",
		ContentSHA256: strings.Repeat("a", 64), SchemaSHA256: strings.Repeat("b", 64),
		GeneratedAt: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC),
		ValidUntil:  time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), Pages: []byte(`[{"complete":true}]`),
		Resources: []Resource{{"workload", "provider", "w1", "1", strings.Repeat("c", 64), "ACTIVE"}},
	}
}

func TestSnapshotRejectsInvalidIdentityAndState(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(*Evidence){
		"scope":     func(e *Evidence) { e.Scope.PrincipalID = "" },
		"nul":       func(e *Evidence) { e.Resources[0].ID = "a\x00b" },
		"duplicate": func(e *Evidence) { e.Resources = append(e.Resources, e.Resources[0]) },
		"state":     func(e *Evidence) { e.Resources[0].State = "APPROVED" },
		"digest":    func(e *Evidence) { e.ContentSHA256 = "invalid" },
		"pages":     func(e *Evidence) { e.Pages = []byte(`[]`) },
		"window":    func(e *Evidence) { e.ValidUntil = e.GeneratedAt },
	} {
		t.Run(name, func(t *testing.T) {
			data := evidence()
			change(&data)
			if _, err := NewSnapshot(data); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid evidence accepted: %v", err)
			}
		})
	}
}

func TestSnapshotNormalizesPrecisionAndProtectsResources(t *testing.T) {
	t.Parallel()
	data := evidence()
	data.GeneratedAt = data.GeneratedAt.Add(123456789 * time.Nanosecond)
	snapshot, err := NewSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	data.Resources[0].State = "RETIRED"
	if snapshot.Evidence().Resources[0].State != "ACTIVE" || snapshot.Evidence().GeneratedAt.Nanosecond() != 123456000 {
		t.Fatal("mutable resource or non-durable timestamp precision")
	}
	if !snapshot.Fresh(snapshot.Evidence().GeneratedAt) || snapshot.Fresh(time.Time{}) || (Snapshot{}).Fresh(data.GeneratedAt) {
		t.Fatal("invalid freshness boundary")
	}
}
