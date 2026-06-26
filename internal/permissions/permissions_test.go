package permissions

import (
	"slices"
	"strings"
	"testing"
)

func TestAllPermissions_NoDuplicates(t *testing.T) {
	seen := make(map[Code]struct{})
	for _, p := range AllPermissions() {
		if _, dup := seen[p]; dup {
			t.Fatalf("duplicate permission code in AllPermissions: %q", p)
		}
		seen[p] = struct{}{}
	}
}

func TestAllPermissions_NonEmptyAndDomainColonFormat(t *testing.T) {
	for _, p := range AllPermissions() {
		s := string(p)
		if s == "" {
			t.Fatalf("empty permission code found")
		}
		if !strings.Contains(s, ":") {
			t.Fatalf("permission %q does not follow domain:action format", s)
		}
	}
}

func TestAllPermissions_IncludesEveryDomainGroup(t *testing.T) {
	all := AllPermissions()
	groups := map[string][]Code{
		"audit": AuditPermissions(),
	}
	for domain, group := range groups {
		if len(group) == 0 {
			t.Fatalf("domain %q has no permissions", domain)
		}
		for _, p := range group {
			if !slices.Contains(all, p) {
				t.Fatalf("AllPermissions() missing %q from domain %q", p, domain)
			}
		}
	}
}
