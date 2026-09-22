package player

import "testing"

func TestPlayerValidate(t *testing.T) {
	if err := (Player{ID: "p1", Elo: 1200}).Validate(); err != nil {
		t.Fatalf("expected valid player, got %v", err)
	}
	if err := (Player{Elo: 1200}).Validate(); err == nil {
		t.Fatal("expected missing id to fail")
	}
}
