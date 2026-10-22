package auth

import (
	"bytes"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "valid", password: "correct horse battery"},
		{name: "too short", password: "short", wantErr: true},
	}
	params := Params{MemoryKiB: 8 * 1024, Iterations: 1, Threads: 1, SaltLen: 16, KeyLen: 32}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password, params)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("hash: %v", err)
			}
			if err := VerifyPassword(hash, tt.password); err != nil {
				t.Fatalf("verify correct password: %v", err)
			}
			if err := VerifyPassword(hash, tt.password+"x"); err == nil {
				t.Fatal("wrong password verified")
			}
		})
	}
}

func TestTokenHashAndEmail(t *testing.T) {
	tok, err := NewToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok.Raw) == 0 || len(tok.Hash) == 0 {
		t.Fatal("empty token")
	}
	if !bytes.Equal(HashToken(tok.Raw), tok.Hash) {
		t.Fatal("token hash does not match")
	}

	emails := []struct {
		in    string
		valid bool
		norm  string
	}{
		{in: "  User@Example.COM ", valid: true, norm: "user@example.com"},
		{in: "no-at-sign", valid: false},
		{in: "a@b", valid: false},
		{in: "a@b.c", valid: true, norm: "a@b.c"},
	}
	for _, e := range emails {
		err := ValidateEmail(e.in)
		if (err == nil) != e.valid {
			t.Errorf("email %q valid=%v err=%v", e.in, e.valid, err)
		}
		if e.valid && NormalizeEmail(e.in) != e.norm {
			t.Errorf("normalize %q = %q", e.in, NormalizeEmail(e.in))
		}
	}
}
