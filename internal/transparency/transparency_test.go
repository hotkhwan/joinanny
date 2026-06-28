package transparency

import "testing"

func TestLabelForMode(t *testing.T) {
	cases := map[string]string{
		"binance_testnet": LabelTestnet,
		"binance_live":    LabelLive,
		"dry_run":         LabelPaper,
		"":                LabelPaper,
		"weird":           LabelPaper,
	}
	for mode, want := range cases {
		if got := LabelForMode(mode); got != want {
			t.Errorf("LabelForMode(%q) = %q, want %q", mode, got, want)
		}
	}
	if IsReal(LabelPaper) || !IsReal(LabelTestnet) || !IsReal(LabelLive) {
		t.Fatal("IsReal classification wrong")
	}
}

func TestHashDeterministicAndSensitive(t *testing.T) {
	a := Hash("BTCUSDT", "long", "1.84")
	if a != Hash("BTCUSDT", "long", "1.84") {
		t.Fatal("hash not deterministic")
	}
	if a == Hash("BTCUSDT", "long", "1.85") {
		t.Fatal("hash insensitive to a field change")
	}
	// Field-boundary safety: separator prevents "ab|c" colliding with "a|bc".
	if Hash("ab", "c") == Hash("a", "bc") {
		t.Fatal("hash boundary collision")
	}
	if len(a) != 64 {
		t.Fatalf("hash length = %d, want 64 hex chars", len(a))
	}
}

func TestMerkleRoot(t *testing.T) {
	if MerkleRoot(nil) != "" {
		t.Fatal("empty leaves should give empty root")
	}
	leaf := Hash("only")
	if MerkleRoot([]string{leaf}) != leaf {
		t.Fatal("single leaf root should equal the leaf")
	}
	// Deterministic and order-sensitive.
	l := []string{Hash("a"), Hash("b"), Hash("c")}
	root := MerkleRoot(l)
	if root == "" || root != MerkleRoot(l) {
		t.Fatal("merkle root not stable")
	}
	rev := []string{l[2], l[1], l[0]}
	if MerkleRoot(rev) == root {
		t.Fatal("merkle root should depend on order")
	}
	// Odd level (3 leaves) duplicates the last: root == hash(hash(a,b), hash(c,c)).
	want := Hash(Hash(l[0], l[1]), Hash(l[2], l[2]))
	if root != want {
		t.Fatalf("merkle root = %s, want %s", root, want)
	}
}
