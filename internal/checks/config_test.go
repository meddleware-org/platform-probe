package checks

import (
	"os"
	"path/filepath"
	"testing"
)

func loadFromString(t *testing.T, body string) ([]Check, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checks.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadFile(path)
}

func TestLoadFileValidPreservesOrder(t *testing.T) {
	checks, err := loadFromString(t, `{
      "groups": [
        {"name": "Identity & Access", "components": [
          {"name": "Sign-in", "url": "http://a/health", "expect_status": 200},
          {"name": "Authorization", "url": "http://b/health", "expect_status": 200}]},
        {"name": "Container Registry", "components": [
          {"name": "Registry API", "url": "http://c/v2/_catalog", "expect_status": 200, "expect_body_contains": "repositories"}]}
      ]
    }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checks) != 3 {
		t.Fatalf("checks = %d, want 3", len(checks))
	}
	if checks[0].Group != "Identity & Access" || checks[0].Name != "Sign-in" {
		t.Errorf("first check = %+v", checks[0])
	}
	if checks[2].Group != "Container Registry" || checks[2].ExpectBodyContains != "repositories" {
		t.Errorf("third check = %+v", checks[2])
	}
}

func TestLoadFileErrors(t *testing.T) {
	cases := map[string]string{
		"malformed json":         `{ not json`,
		"no groups":              `{"groups": []}`,
		"group without name":     `{"groups":[{"name":"","components":[{"name":"x","url":"http://a"}]}]}`,
		"group without members":  `{"groups":[{"name":"G","components":[]}]}`,
		"component without name": `{"groups":[{"name":"G","components":[{"name":"","url":"http://a"}]}]}`,
		"component without url":  `{"groups":[{"name":"G","components":[{"name":"x","url":""}]}]}`,
		"unknown field":          `{"groups":[{"name":"G","components":[{"name":"x","url":"http://a","oops":1}]}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadFromString(t, body); err == nil {
				t.Errorf("expected an error for %s, got nil", name)
			}
		})
	}
}

func TestLoadFileMissing(t *testing.T) {
	if _, err := LoadFile(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Error("expected an error for a missing file")
	}
}
