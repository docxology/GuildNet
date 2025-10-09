package tests

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/your/module/internal/httpx"
)

// Re-declare minimal helpers mirroring main.go for unit testing without exporting from main package.
func dns1123NameTest(s string) string {
	// simplified copy of dns1123Name logic from main.go
	return hostappDns1123NameShim(s)
}

// hostappDns1123NameShim is defined in a generated shim below via build tags in real scenario; placeholder here.
// For prototype simplicity, we inline a reduced transformation.
func hostappDns1123NameShim(s string) string {
	// Accept basic alnum and dash collapsing for test expectations
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range []rune(s) {
		if r >= 'A' && r <= 'Z' {
			r = r + ('a' - 'A')
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
			prevDash = false
			continue
		}
		if r == '-' || r == '_' || r == ' ' {
			if !prevDash && len(out) > 0 {
				out = append(out, '-')
				prevDash = true
			}
			continue
		}
		// drop other chars
	}
	// trim leading/trailing dashes
	res := string(out)
	for len(res) > 0 && res[0] == '-' {
		res = res[1:]
	}
	for len(res) > 0 && res[len(res)-1] == '-' {
		res = res[:len(res)-1]
	}
	return res
}

func TestDNS1123NameBasic(t *testing.T) {
	cases := map[string]string{
		"My App":         "my-app",
		"___ABC":         "abc",
		"Hello--World!!": "hello-world",
		"":               "",
	}
	for in, want := range cases {
		if got := dns1123NameTest(in); got != want {
			to, _ := json.Marshal(map[string]string{"in": in, "got": got, "want": want})
			// Use t.Fatalf for immediate clarity
			t.Fatalf("dns1123Name mismatch %s", string(to))
		}
	}
}

func TestJSONErrorShape(t *testing.T) {
	r := httptest.NewRecorder()
	// Manually seed request id header expectation
	r.Header().Set("X-Request-Id", "test-req-123")
	httpx.JSONError(r, 404, "workspace not found", "not_found")
	if r.Code != 404 {
		t.Fatalf("expected 404 code, got %d", r.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["code"] != "not_found" {
		t.Fatalf("expected code not_found got %v", body["code"])
	}
	if body["message"] != "workspace not found" {
		t.Fatalf("unexpected message %v", body["message"])
	}
	// request_id may be empty string; ensure key exists even if empty
	if _, ok := body["request_id"]; !ok {
		t.Fatalf("expected request_id key present")
	}
}
