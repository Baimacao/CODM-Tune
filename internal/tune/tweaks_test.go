package tune

import "testing"

// Every shipped tweak must be reversible: a definition with no Sets cannot be
// snapshotted, and reverting it would silently do nothing.
func TestTweakDefsAreReversible(t *testing.T) {
	c := NewControl()
	defs := c.TweakDefs()
	if len(defs) < 8 {
		t.Fatalf("expected the full tweak set, got %d", len(defs))
	}
	for id, def := range defs {
		if def.ID != id {
			t.Errorf("%s: ID field is %q", id, def.ID)
		}
		if def.Name == "" || def.Desc == "" {
			t.Errorf("%s: missing name or description", id)
		}
		if len(def.Sets) == 0 {
			t.Errorf("%s: no registry sets, so it cannot be reverted", id)
		}
		for _, s := range def.Sets {
			if s.Key == "" || s.Name == "" {
				t.Errorf("%s: set with empty key or value name", id)
			}
			switch s.Kind {
			case "dword":
			case "sz":
				if s.OnS == "" {
					t.Errorf("%s: string set with empty value", id)
				}
			default:
				t.Errorf("%s: unknown kind %q", id, s.Kind)
			}
		}
	}
}

// The filesystem group has to stay opt-in. A tweak that ships on by default is
// exactly how v12.4 ended up purging texture caches nobody asked it to touch.
func TestFilesystemTweaksDefaultOff(t *testing.T) {
	c := NewControl()
	for _, id := range []string{"ntfsmem", "longpaths", "prefetch", "gtr"} {
		if _, ok := c.TweakDefs()[id]; !ok {
			t.Fatalf("%s is missing from the tweak set", id)
		}
		if c.TweakOn(id) {
			t.Errorf("%s must default to off", id)
		}
	}
}

// A fresh profile must not schedule reclamation.
func TestFreshConfigLeavesReclamationOff(t *testing.T) {
	if defaultConfig().MemAuto {
		t.Fatal("automatic reclamation must be off in a fresh profile")
	}
}
