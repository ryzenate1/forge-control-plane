package auth

import (
	"testing"
)

func TestExtendedScopeSetHas(t *testing.T) {
	s := NewExtendedScopeSet(ServerRead, ServerWrite)
	if !s.Has(ServerRead) {
		t.Error("expected Has(ServerRead) to be true")
	}
	if s.Has(ServerAdmin) {
		t.Error("expected Has(ServerAdmin) to be false")
	}
}

func TestExtendedScopeSetAddRemove(t *testing.T) {
	s := NewExtendedScopeSet()
	s.Add(ServerRead)
	if !s.Has(ServerRead) {
		t.Error("expected Has after Add")
	}
	s.Remove(ServerRead)
	if s.Has(ServerRead) {
		t.Error("expected not Has after Remove")
	}
}

func TestExtendedScopeSetMerge(t *testing.T) {
	a := NewExtendedScopeSet(ServerRead, ServerWrite)
	b := NewExtendedScopeSet(UserRead, UserWrite)
	merged := a.Merge(b)

	for _, scope := range []string{ServerRead, ServerWrite, UserRead, UserWrite} {
		if !merged.Has(scope) {
			t.Errorf("expected merged set to have %q", scope)
		}
	}
	if len(merged) != 4 {
		t.Errorf("expected 4 scopes, got %d", len(merged))
	}
}

func TestExtendedScopeSetIntersect(t *testing.T) {
	a := NewExtendedScopeSet(ServerRead, ServerWrite, UserRead)
	b := NewExtendedScopeSet(ServerRead, UserRead, NodeRead)
	result := a.Intersect(b)

	if !result.Has(ServerRead) {
		t.Error("expected ServerRead in intersection")
	}
	if !result.Has(UserRead) {
		t.Error("expected UserRead in intersection")
	}
	if result.Has(ServerWrite) {
		t.Error("expected ServerWrite not in intersection")
	}
	if result.Has(NodeRead) {
		t.Error("expected NodeRead not in intersection")
	}
}

func TestExtendedScopeSetIsSubsetOf(t *testing.T) {
	small := NewExtendedScopeSet(ServerRead)
	big := NewExtendedScopeSet(ServerRead, ServerWrite, UserRead)

	if !small.IsSubsetOf(big) {
		t.Error("expected small to be subset of big")
	}
	if big.IsSubsetOf(small) {
		t.Error("expected big not to be subset of small")
	}
}

func TestExtendedScopeSetSlice(t *testing.T) {
	s := NewExtendedScopeSet(ServerWrite, ServerRead)
	slice := s.Slice()
	if len(slice) != 2 {
		t.Fatalf("expected 2 items, got %d", len(slice))
	}
	if slice[0] != ServerRead {
		t.Errorf("expected first item %q, got %q", ServerRead, slice[0])
	}
}

func TestExtendedScopeSetString(t *testing.T) {
	s := NewExtendedScopeSet(UserRead, ServerRead)
	str := s.String()
	if str == "" {
		t.Error("expected non-empty string")
	}
}

func TestCheckScope(t *testing.T) {
	scopes := NewExtendedScopeSet(ServerRead, ServerWrite)
	if !CheckScope(scopes, ServerRead) {
		t.Error("expected CheckScope to return true for ServerRead")
	}
	if CheckScope(scopes, ServerAdmin) {
		t.Error("expected CheckScope to return false for ServerAdmin")
	}
}

func TestCheckScopeWildcard(t *testing.T) {
	scopes := NewExtendedScopeSet("server:*")
	if !CheckScope(scopes, ServerRead) {
		t.Error("expected wildcard to match ServerRead")
	}
	if !CheckScope(scopes, ServerWrite) {
		t.Error("expected wildcard to match ServerWrite")
	}
	if CheckScope(scopes, UserRead) {
		t.Error("expected wildcard not to match UserRead")
	}
}

func TestExpandScopes(t *testing.T) {
	expanded := ExpandScopes([]string{"server:*"})
	if !expanded.Has(ServerRead) {
		t.Error("expected server:* to expand to include ServerRead")
	}
	if !expanded.Has(ServerWrite) {
		t.Error("expected server:* to expand to include ServerWrite")
	}
	if !expanded.Has(ServerAdmin) {
		t.Error("expected server:* to expand to include ServerAdmin")
	}
	if !expanded.Has(ServerConsole) {
		t.Error("expected server:* to expand to include ServerConsole")
	}
	if !expanded.Has(ServerFiles) {
		t.Error("expected server:* to expand to include ServerFiles")
	}
	if !expanded.Has(ServerBackups) {
		t.Error("expected server:* to expand to include ServerBackups")
	}
	if expanded.Has(UserRead) {
		t.Error("expected server:* not to include UserRead")
	}
}

func TestExpandScopesMultiple(t *testing.T) {
	expanded := ExpandScopes([]string{"server:*", "user:read"})
	if !expanded.Has(ServerRead) {
		t.Error("expected ServerRead")
	}
	if !expanded.Has(UserRead) {
		t.Error("expected UserRead")
	}
	if expanded.Has(UserAdmin) {
		t.Error("expected not UserAdmin")
	}
}

func TestExpandScopesExplicit(t *testing.T) {
	expanded := ExpandScopes([]string{ServerRead, UserWrite})
	if !expanded.Has(ServerRead) {
		t.Error("expected ServerRead")
	}
	if !expanded.Has(UserWrite) {
		t.Error("expected UserWrite")
	}
	if expanded.Has(ServerWrite) {
		t.Error("expected not ServerWrite")
	}
}

func TestExpandScopesEmpty(t *testing.T) {
	expanded := ExpandScopes([]string{})
	if len(expanded) != 0 {
		t.Errorf("expected empty set, got %d items", len(expanded))
	}
}

func TestExpandScopesWhitespace(t *testing.T) {
	expanded := ExpandScopes([]string{"", "  ", ServerRead})
	if !expanded.Has(ServerRead) {
		t.Error("expected ServerRead")
	}
	if len(expanded) != 1 {
		t.Errorf("expected 1 scope, got %d", len(expanded))
	}
}

func TestRoleScopes(t *testing.T) {
	adminScopes, ok := RoleScopes["admin"]
	if !ok {
		t.Fatal("expected admin role in RoleScopes")
	}
	for _, scope := range AllExtendedScopes {
		if !adminScopes.Has(scope) {
			t.Errorf("expected admin to have scope %q", scope)
		}
	}

	userScopes, ok := RoleScopes["user"]
	if !ok {
		t.Fatal("expected user role in RoleScopes")
	}
	if !userScopes.Has(ServerRead) {
		t.Error("expected user to have ServerRead")
	}
	if userScopes.Has(ServerAdmin) {
		t.Error("expected user not to have ServerAdmin")
	}
	if userScopes.Has(SystemAdmin) {
		t.Error("expected user not to have SystemAdmin")
	}
}

func TestAllExtendedScopesConstants(t *testing.T) {
	expected := []string{
		ServerRead, ServerWrite, ServerAdmin, ServerConsole, ServerFiles, ServerBackups,
		UserRead, UserWrite, UserAdmin,
		NodeRead, NodeWrite, NodeAdmin,
		SystemRead, SystemAdmin, SystemAudit,
	}
	if len(AllExtendedScopes) != len(expected) {
		t.Errorf("expected %d scopes, got %d", len(expected), len(AllExtendedScopes))
	}
}

func TestExpandScopesDotWildcard(t *testing.T) {
	expanded := ExpandScopes([]string{"node.*"})
	if !expanded.Has(NodeRead) {
		t.Error("expected node.* to expand to include NodeRead")
	}
	if !expanded.Has(NodeWrite) {
		t.Error("expected node.* to expand to include NodeWrite")
	}
	if !expanded.Has(NodeAdmin) {
		t.Error("expected node.* to expand to include NodeAdmin")
	}
	if expanded.Has(ServerRead) {
		t.Error("expected node.* not to include ServerRead")
	}
}
