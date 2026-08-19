package providers

import "testing"

// The oracle slug is host|siteNumber because neither can be derived from the
// other; a malformed slug must fail loudly rather than build a nonsense URL.
func TestOracleSlugValidation(t *testing.T) {
	for _, bad := range []string{"", "hostonly", "|CX_1001", "host|"} {
		if _, err := fetchOracle(t.Context(), bad); err == nil {
			t.Errorf("fetchOracle(%q) should reject a malformed slug", bad)
		}
	}
}
