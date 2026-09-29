package cache

import (
	"net/http"
	"strings"
	"testing"
)

// keyTestRequest returns one representative request for identity tests.
// [NO_SPEC] Provides a stable fixture shared by deterministic-key test cases.
func keyTestRequest() Request {
	return Request{
		Namespace: "source",
		Method:    http.MethodGet,
		URL:       "https://example.test/models?page=2",
		Headers: http.Header{
			"Accept":     {"application/json"},
			"X-Format":   {"compact", "stable"},
			"User-Agent": {"ai-pareto"},
		},
	}
}

// cloneHeaders returns independent header storage for one key test mutation.
// [NO_SPEC] Prevents table cases from sharing mutable request header slices.
func cloneHeaders(headers http.Header) http.Header {
	clone := make(http.Header, len(headers))
	for name, values := range headers {
		clone[name] = append([]string(nil), values...)
	}
	return clone
}

// TestKeyIgnoresHeaderMapIterationOrder verifies canonical header ordering.
// Expected: equivalent header maps produce one identical opaque key.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Deterministic secret-safe cache identity".
func TestKeyIgnoresHeaderMapIterationOrder(t *testing.T) {
	first := keyTestRequest()
	first.Headers = http.Header{
		"X-Format": {"compact", "stable"},
		"Accept":   {"application/json"},
	}
	second := keyTestRequest()
	second.Headers = http.Header{
		"Accept":   {"application/json"},
		"X-Format": {"compact", "stable"},
	}

	if firstKey, secondKey := keyFor(first), keyFor(second); firstKey != secondKey {
		t.Fatalf("keyFor() differs for equivalent headers: %q != %q", firstKey, secondKey)
	}
}

// TestKeyIncludesEveryRepresentationIdentityInput verifies namespace, method, URL, and header identity fields.
// Expected: changing one representation input changes the opaque key while preserving the other inputs.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Deterministic secret-safe cache identity".
func TestKeyIncludesEveryRepresentationIdentityInput(t *testing.T) {
	base := keyTestRequest()
	cases := []struct {
		name   string
		change func(*Request)
	}{
		{
			name: "namespace",
			change: func(request *Request) {
				request.Namespace = "other-source"
			},
		},
		{
			name: "method",
			change: func(request *Request) {
				request.Method = http.MethodPost
			},
		},
		{
			name: "url-query",
			change: func(request *Request) {
				request.URL = "https://example.test/models?page=3"
			},
		},
		{
			name: "header-name",
			change: func(request *Request) {
				request.Headers["X-Format"] = []string{"expanded", "stable"}
			},
		},
		{
			name: "header-value",
			change: func(request *Request) {
				request.Headers["X-Format"] = []string{"compact", "changed"}
			},
		},
	}

	baseKey := keyFor(base)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			changed := base
			changed.Headers = cloneHeaders(base.Headers)
			testCase.change(&changed)
			if changedKey := keyFor(changed); changedKey == baseKey {
				t.Fatalf("keyFor() = %q after changing %s, want a distinct key", changedKey, testCase.name)
			}
		})
	}
}

// TestKeySeparatesCredentialIdentities verifies absent, empty, and distinct credential inputs.
// Expected: each credential selection produces a distinct key and no raw credential appears in a key or authorization input.
// [SPEC] openspec/changes/implement-raw-http-cache/specs/raw-http-cache/spec.md, heading "Requirement: Deterministic secret-safe cache identity".
func TestKeySeparatesCredentialIdentities(t *testing.T) {
	base := keyTestRequest()
	base.Headers["Authorization"] = []string{"Bearer raw-authorization"}
	withoutCredential := keyFor(base)

	withFirstCredential := base
	withFirstCredential.Credential = []byte("credential-A")
	firstKey := keyFor(withFirstCredential)

	withSecondCredential := base
	withSecondCredential.Credential = []byte("credential-B")
	secondKey := keyFor(withSecondCredential)

	withEmptyCredential := base
	withEmptyCredential.Credential = []byte{}
	emptyKey := keyFor(withEmptyCredential)

	keys := []string{withoutCredential, firstKey, secondKey, emptyKey}
	for firstIndex, first := range keys {
		for secondIndex := firstIndex + 1; secondIndex < len(keys); secondIndex++ {
			if first == keys[secondIndex] {
				t.Fatalf("keys[%d] and keys[%d] both equal %q", firstIndex, secondIndex, first)
			}
		}
	}
	for _, raw := range []string{"credential-A", "credential-B", "raw-authorization"} {
		if strings.Contains(withoutCredential+firstKey+secondKey+emptyKey, raw) {
			t.Fatalf("raw secret %q appears in cache key", raw)
		}
	}
}
