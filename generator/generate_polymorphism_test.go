package generator

import "testing"

func TestResolvePolymorphismSeedUsesProvidedSeed(t *testing.T) {
	got := resolvePolymorphismSeed(12345)
	if got != 12345 {
		t.Fatalf("expected provided seed 12345, got %d", got)
	}
}

func TestResolvePolymorphismSeedUsesTimestampWhenMissing(t *testing.T) {
	got := resolvePolymorphismSeed(0)
	if got == 0 {
		t.Fatalf("expected non-zero timestamp seed when seed is omitted")
	}
}
