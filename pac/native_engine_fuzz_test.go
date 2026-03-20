package pac

import (
	"strings"
	"testing"
)

// FuzzDomainSuffixTrie tests insertion and matching consistency for domain suffix trie.
func FuzzDomainSuffixTrie(f *testing.F) {
	// Seed with simple cases
	f.Add("example.com", 0)
	f.Add("test.com", 1)
	f.Fuzz(func(t *testing.T, pattern string, index int) {
		// Normalize
		pattern = strings.ToLower(pattern)
		pattern = strings.TrimPrefix(pattern, ".")
		if pattern == "" {
			return
		}
		trie := newDomainSuffixTrie()
		meta := ruleMeta{index: index, route: Route{Strategy: "TUN"}}
		trie.insert(pattern, meta)

		// The pattern itself should match
		got := trie.match(pattern, false)
		if got == nil {
			t.Fatalf("pattern %q did not match itself", pattern)
		}
		if got.index != index {
			t.Fatalf("expected index %d, got %d", index, got.index)
		}

		// Insert the same pattern with a higher index; earlier index should win
		if index2 := index + 1; index2 > index {
			meta2 := ruleMeta{index: index2, route: Route{Strategy: "DIRECT"}}
			trie.insert(pattern, meta2)
			got = trie.match(pattern, false)
			if got.index != index {
				t.Fatalf("after duplicate with higher index, expected %d, got %d", index, got.index)
			}
		}
	})
}
