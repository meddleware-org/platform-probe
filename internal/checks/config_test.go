// Tests for LoadFile: the checks file is operator-edited, so each way it can be wrong must fail loudly and name what to fix.

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadFromString writes body to a temporary checks file and loads it, so each test states its file inline.
func loadFromString(t *testing.T, body string) ([]Check, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checks.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadFile(path)
}

// TestLoadFileValidPreservesOrder checks a valid file flattens to the checks in group order, then component order.
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

// TestLoadFileErrors checks every kind of bad file is refused: malformed JSON, missing names, URLs, unknown keys, out-of-range statuses and unusable URLs.
func TestLoadFileErrors(t *testing.T) {
	// Each name describes one way an operator's edit can go wrong; every one must be an error.
	cases := map[string]string{
		"malformed json":          `{ not json`,
		"no groups":               `{"groups": []}`,
		"group without name":      `{"groups":[{"name":"","components":[{"name":"x","url":"http://a"}]}]}`,
		"group without members":   `{"groups":[{"name":"G","components":[]}]}`,
		"component without name":  `{"groups":[{"name":"G","components":[{"name":"","url":"http://a"}]}]}`,
		"component without url":   `{"groups":[{"name":"G","components":[{"name":"x","url":""}]}]}`,
		"unknown field":           `{"groups":[{"name":"G","components":[{"name":"x","url":"http://a","oops":1}]}]}`,
		"status below 100":        `{"groups":[{"name":"G","components":[{"name":"x","url":"http://a","expect_status":99}]}]}`,
		"status above 599":        `{"groups":[{"name":"G","components":[{"name":"x","url":"http://a","expect_status":600}]}]}`,
		"negative status":         `{"groups":[{"name":"G","components":[{"name":"x","url":"http://a","expect_status":-1}]}]}`,
		"url with no scheme":      `{"groups":[{"name":"G","components":[{"name":"x","url":"a.example/health"}]}]}`,
		"url with another scheme": `{"groups":[{"name":"G","components":[{"name":"x","url":"ftp://a.example/health"}]}]}`,
		"url with no host":        `{"groups":[{"name":"G","components":[{"name":"x","url":"https://"}]}]}`,
		"url that does not parse": `{"groups":[{"name":"G","components":[{"name":"x","url":"https://[::1"}]}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadFromString(t, body); err == nil {
				t.Errorf("expected an error for %s, got nil", name)
			}
		})
	}
}

// TestLoadFileMissing checks a file that does not exist is an error, not an empty configuration.
func TestLoadFileMissing(t *testing.T) {
	if _, err := LoadFile(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Error("expected an error for a missing file")
	}
}

// TestLoadFileExpectStatus checks the expected status survives loading, with 0 meaning "any 2xx".
func TestLoadFileExpectStatus(t *testing.T) {
	checks, err := loadFromString(t, `{"groups":[{"name":"G","components":[
      {"name":"omitted","url":"http://a"},
      {"name":"ok","url":"http://b","expect_status":200},
      {"name":"redirect","url":"http://c","expect_status":302},
      {"name":"edge","url":"http://d","expect_status":599}]}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The expected codes in file order: omitted, a plain 200, a redirect code a check may expect, and the top of the valid range.
	want := []int{0, 200, 302, 599}
	for i, w := range want {
		if checks[i].ExpectStatus != w {
			t.Errorf("check %d (%s): ExpectStatus = %d, want %d", i, checks[i].Name, checks[i].ExpectStatus, w)
		}
	}
}

// TestLoadFileBadURLNeverEchoed checks the rejection of an unusable URL names the component and repeats no part of the
// value: a configured URL can carry a user name, password, path token and query key.
func TestLoadFileBadURLNeverEchoed(t *testing.T) {
	_, err := loadFromString(t, `{"groups":[{"name":"G","components":[{"name":"svc","url":"ftp://user:s3cret@host.example/tok3n?key=k3y"}]}]}`)
	if err == nil {
		t.Fatal("expected an error for an ftp URL")
	}
	for _, part := range []string{"user", "s3cret", "host.example", "tok3n", "k3y", "ftp"} {
		if strings.Contains(err.Error(), part) {
			t.Errorf("error %q repeats %q", err, part)
		}
	}
	if !strings.Contains(err.Error(), `"svc"`) {
		t.Errorf("error %q does not name the component", err)
	}
}
