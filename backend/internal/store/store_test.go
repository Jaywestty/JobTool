package store

import "testing"

// The migration files must be embedded; if the embed directive breaks, the
// binary would ship without schema.
func TestMigrationsEmbedded(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded migrations")
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.Name()] {
			t.Fatalf("duplicate migration %s", e.Name())
		}
		seen[e.Name()] = true
	}
}
