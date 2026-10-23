package perfeng

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

var fixtureScope = Expectations{
	Issuer: "https://provider.example/identity", PrincipalIssuer: "https://auth.example/tenant",
	PrincipalID: "argus-service", ProducerID: "perfeng-control-plane", ProducerVersion: "1.0.0",
}

var fixtureTime = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	data, err := os.ReadFile("snapshot/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(data, &pages); err != nil {
		t.Fatal(err)
	}

	return pages
}

func TestPublishedArtifactsRemainPinned(t *testing.T) {
	for name, want := range map[string]string{
		"catalog.schema.json": "becf420e60231bd0411352ba94b63c8450aa1abb0cf569c0e1634915eb19ce75",
		"pages.json":          "f07a6d6f1b8a3ba02692acd615f636d0300fcf9fe7f434f4bb12f185bebd1e30",
		"empty.json":          "88ddd468009e239061897dbc39ac30525c45e9f8e0f163ec6b0fcdedecefb97c",
	} {
		data, err := os.ReadFile("snapshot/" + name)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != want {
			t.Errorf("%s differs from reviewed upstream bytes", name)
		}
	}
}

func TestPublishedSnapshots(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"pages", "empty"} {
		pages := fixture(t, name)
		original := make([]json.RawMessage, len(pages))
		for index, page := range pages {
			original[index] = slices.Clone(page)
		}
		for range 2 {
			if err := Validate(pages, fixtureScope, fixtureTime); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if !reflect.DeepEqual(original, pages) {
			t.Fatal("validator mutated caller evidence")
		}
	}
}

func mutate(t *testing.T, change func([]map[string]any), rehash bool) []json.RawMessage {
	t.Helper()
	var pages []map[string]any
	data, err := json.Marshal(fixture(t, "pages"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &pages); err != nil {
		t.Fatal(err)
	}
	change(pages)
	if rehash {
		items := make([]any, 0)
		for _, page := range pages {
			items = append(items, page["items"].([]any)...)
		}
		digest := sha256.Sum256(canonical(items))
		for _, page := range pages {
			snapshot := object(page["snapshot"])
			object(snapshot["reference"])["sha256"] = hex.EncodeToString(digest[:])
			snapshot["totalItems"] = len(items)
		}
	}
	result := make([]json.RawMessage, 0, len(pages))
	for _, page := range pages {
		data, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, data)
	}

	return result
}

func TestRejectsIncompleteAndWronglyScopedSnapshots(t *testing.T) {
	changes := map[string]func([]map[string]any){
		"mixed metadata": func(p []map[string]any) { object(p[1]["snapshot"])["validUntil"] = "2026-10-22T14:00:00Z" },
		"wrong count": func(p []map[string]any) {
			for _, v := range p {
				object(v["snapshot"])["totalItems"] = 9
			}
		},
		"broken cursor":          func(p []map[string]any) { object(p[1]["page"])["cursor"] = "other" },
		"nonfinal null cursor":   func(p []map[string]any) { object(p[0]["page"])["nextCursor"] = nil },
		"noninitial null cursor": func(p []map[string]any) { object(p[1]["page"])["cursor"] = nil },
		"wrong page order":       func(p []map[string]any) { object(p[1]["page"])["index"] = 0 },
		"partial flag":           func(p []map[string]any) { object(p[0]["snapshot"])["complete"] = false },
		"unknown field":          func(p []map[string]any) { p[0]["secret"] = "must not appear in errors" },
		"digest":                 func(p []map[string]any) { object(p[0]["items"].([]any)[0])["displayName"] = "changed" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			if err := Validate(mutate(t, change, false), fixtureScope, fixtureTime); err == nil {
				t.Fatal("accepted invalid snapshot")
			} else if strings.Contains(err.Error(), "must not appear") {
				t.Fatal("reflected untrusted payload into error")
			}
		})
	}
	pages := fixture(t, "pages")
	for _, partial := range [][]json.RawMessage{nil, pages[:1], {pages[1], pages[0]}, {pages[0], pages[0]}} {
		if err := Validate(partial, fixtureScope, fixtureTime); err == nil {
			t.Fatal("accepted missing or repeated pages")
		}
	}
	for _, field := range []string{"Issuer", "PrincipalIssuer", "PrincipalID", "ProducerID", "ProducerVersion"} {
		scope := fixtureScope
		reflect.ValueOf(&scope).Elem().FieldByName(field).SetString("unexpected")
		if err := Validate(pages, scope, fixtureTime); err == nil {
			t.Fatalf("trusted %s from body", field)
		}
	}
	for _, at := range []time.Time{{}, fixtureTime.Add(-2 * time.Hour), fixtureTime.Add(time.Hour)} {
		if err := Validate(pages, fixtureScope, at); err == nil {
			t.Fatal("accepted missing, future or expired observation time")
		}
	}
}

func TestReferenceClosureAndLifecycle(t *testing.T) {
	changes := map[string]func([]any){
		"retired route":          func(items []any) { object(items[4])["state"] = "RETIRED" },
		"missing reference":      func(items []any) { object(object(items[0])["ownerRoute"])["id"] = "absent" },
		"substituted revision":   func(items []any) { object(object(items[0])["ownerRoute"])["revision"] = "2" },
		"retired profile":        func(items []any) { object(items[1])["state"] = "RETIRED" },
		"unsupported profile":    func(items []any) { object(items[0])["supportedProfiles"] = []any{} },
		"retired environment":    func(items []any) { object(items[2])["state"] = "RETIRED" },
		"unapproved baseline":    func(items []any) { object(items[5])["state"] = "CANDIDATE" },
		"wrong baseline context": func(items []any) { object(items[5])["environment"] = object(items[5])["policy"] },
		"duplicate identity":     func(items []any) { items[3] = items[2] },
		"duplicate profile": func(items []any) {
			refs := object(items[0])["supportedProfiles"].([]any)
			object(items[0])["supportedProfiles"] = append(refs, refs[0])
		},
		"duplicate selection": func(items []any) {
			selections := object(items[1])["selections"].([]any)
			object(items[1])["selections"] = append(selections, selections[0])
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			pages := mutate(t, func(p []map[string]any) {
				first := len(p[0]["items"].([]any))
				items := append(p[0]["items"].([]any), p[1]["items"].([]any)...)
				change(items)
				p[0]["items"], p[1]["items"] = items[:first], items[first:]
			}, true)
			if err := Validate(pages, fixtureScope, fixtureTime); err == nil {
				t.Fatal("accepted invalid resource graph with valid digest")
			}
		})
	}
	pages := mutate(t, func(p []map[string]any) {
		for _, page := range p {
			for _, value := range page["items"].([]any) {
				object(value)["state"] = "RETIRED"
			}
		}
	}, true)
	if err := Validate(pages, fixtureScope, fixtureTime); err != nil {
		t.Fatalf("rejected explicit retirement history: %v", err)
	}
}

func TestValidAbsenceIssuerScopeAndPagePartition(t *testing.T) {
	pages := mutate(t, func(p []map[string]any) {
		profile := object(p[0]["items"].([]any)[1])
		object(profile["selections"].([]any)[0])["baseline"] = nil
		environment := map[string]any{
			"kind": "environment", "displayName": "another issuer", "state": "ACTIVE",
			"reference": map[string]any{"issuer": "https://other.example", "id": "staging", "revision": "1", "sha256": strings.Repeat("a", 64)},
		}
		p[1]["items"] = append(p[1]["items"].([]any), environment)
	}, true)
	if err := Validate(pages, fixtureScope, fixtureTime.Add(-time.Hour)); err != nil {
		t.Fatalf("rejected null baseline, scoped identity or inclusive generation: %v", err)
	}
	var first, second map[string]any
	if err := json.Unmarshal(pages[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pages[1], &second); err != nil {
		t.Fatal(err)
	}
	first["items"] = append(first["items"].([]any), second["items"].([]any)...)
	object(first["snapshot"])["totalPages"] = 1
	object(first["page"])["nextCursor"] = nil
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate([]json.RawMessage{data}, fixtureScope, fixtureTime); err != nil {
		t.Fatalf("digest depended on page partition: %v", err)
	}
}

func TestCatalogInputLimits(t *testing.T) {
	large := make(json.RawMessage, (2<<20)+1)
	cases := [][]json.RawMessage{{large}, make([]json.RawMessage, 1001), make([]json.RawMessage, 33)}
	for index := range cases[2] {
		cases[2][index] = large[:2<<20]
	}
	for _, pages := range cases {
		if err := Validate(pages, fixtureScope, fixtureTime); err == nil {
			t.Fatal("accepted catalog beyond local input limits")
		}
	}
}

func TestCanonicalStringsAndAmbiguousJSON(t *testing.T) {
	want := "{\"a\":[null,\"é😀<>&/\u2028\u2029\"],\"z\":\"\\b\\f\\n\\r\\t\\u0000\\u001f\\\"\\\\\"}"
	value := map[string]any{"z": "\b\f\n\r\t\x00\x1f\"\\", "a": []any{nil, "é😀<>&/\u2028\u2029"}}
	if got := string(canonical(value)); got != want {
		t.Fatalf("canonical bytes differ: %q != %q", got, want)
	}
	for _, value := range []string{`"\ud800"`, `"\udc00"`, `"\ud800\u0041"`, "\"\xff\""} {
		if validUnicode([]byte(value)) {
			t.Fatalf("accepted invalid Unicode %s", value)
		}
	}
	for _, value := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `"�"`} {
		if !validUnicode([]byte(value)) {
			t.Fatalf("rejected valid Unicode %s", value)
		}
	}
	pages := fixture(t, "empty")
	for _, invalid := range []string{
		strings.Replace(string(pages[0]), `"schemaVersion": 1`, `"schemaVersion": 1, "schemaVersion": 1`, 1),
		string(pages[0]) + ` {}`, strings.Repeat("[", 34) + strings.Repeat("]", 34),
	} {
		if err := Validate([]json.RawMessage{[]byte(invalid)}, fixtureScope, fixtureTime); err == nil {
			t.Fatal("accepted ambiguous, trailing or deeply nested JSON")
		}
	}
}
