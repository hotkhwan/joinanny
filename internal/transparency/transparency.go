// Package transparency provides the tamper-evidence primitives behind ANNY's
// "Flight Recorder": a stable content hash per record and a daily Merkle root
// over those hashes. The root is what gets anchored on-chain at launch (opBNB /
// OpenTimestamps) — until then it is computed and shown so the mechanism is
// real and verifiable, just not yet anchored.
//
// Honesty note: a hash/anchor proves a record was not rewritten after its
// timestamp. It does NOT prove the trade or its PnL is real — that comes from
// the underlying testnet/live execution, labelled per record.
package transparency

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Execution labels for a record. Headline "real" stats count testnet+live only;
// paper (dry-run / simulated goal runs) is shown separately so paper results are
// never passed off as a live track record.
const (
	LabelPaper   = "paper"
	LabelTestnet = "testnet"
	LabelLive    = "live"
)

// LabelForMode maps an order/journal execution mode to a Flight Recorder label.
func LabelForMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "binance_testnet", "testnet":
		return LabelTestnet
	case "binance_live", "live", "real":
		return LabelLive
	default:
		// dry_run, paper, simulation, or anything unknown → paper (not real).
		return LabelPaper
	}
}

// IsReal reports whether a label counts toward the live track record.
func IsReal(label string) bool {
	return label == LabelTestnet || label == LabelLive
}

// Hash returns the hex SHA-256 of the parts joined by a unit separator. Callers
// pass a record's stable fields in a fixed order so the hash is deterministic
// and reproducible by anyone with the raw record.
func Hash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// MerkleRoot computes a binary SHA-256 Merkle root over the given leaf hashes
// (hex strings, in feed order). An odd node at any level is paired with itself
// (the standard duplicate-last convention). Returns "" for no leaves and the
// single leaf unchanged for one. The result is stable for a fixed leaf order,
// so the same day's records always yield the same root.
func MerkleRoot(leaves []string) string {
	if len(leaves) == 0 {
		return ""
	}
	level := append([]string(nil), leaves...)
	for len(level) > 1 {
		next := make([]string, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			left := level[i]
			right := left
			if i+1 < len(level) {
				right = level[i+1]
			}
			next = append(next, Hash(left, right))
		}
		level = next
	}
	return level[0]
}
