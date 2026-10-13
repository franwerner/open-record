package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/franwerner/open-record/internal/finding"
)

// HashCarrier is any JSON document the binary writes inside .openrecord/.
// components.json is the first; actors.json, checks.json, sandbox.json, and
// the implementation/ and runs/ files join it as later issues introduce
// them. Every one of them carries the same `hash` field, stamped and
// verified the same way — this is the one place that formula lives, so a
// later type adopts it instead of inventing it again, exactly as every
// record already shares one BodyHash rather than each kind computing its
// own.
type HashCarrier interface {
	// StoredHash is the hash as read from disk — empty when the field was
	// absent, which VerifyHash treats as "no hash at all" rather than
	// guessing at a difference between absent and empty.
	StoredHash() string
	// WithHash returns a copy of the value carrying the given hash. An
	// empty string excludes the field from serialization via `omitempty`,
	// which is how "this file's content with its own hash excluded" is
	// produced: the same value, serialized once with the field gone.
	WithHash(hash string) any
}

// StampHash is the SHA-256 hex digest a HashCarrier's hash field gets: the
// exact bytes `json.MarshalIndent(value, "", "  ")` produces with the field
// empty (so `omitempty` drops it), plus the single trailing newline every
// file the binary writes ends with. A type's Save computes this, assigns it
// to the field, and marshals a second time with the field now populated —
// which is also why `hash` always lands as the last key: it is the one
// field whose value is not known until everything else has already been
// written.
//
// Reused unchanged by validate and by a writer's integrity check: both
// recompute this from whatever was parsed off disk and compare it against
// what that parse found in the `hash` field.
func StampHash(value HashCarrier) (string, error) {
	raw, err := json.MarshalIndent(value.WithHash(""), "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyHash reports why a JSON file the binary writes cannot be trusted: no
// `hash` field at all (json-hash-missing), or one present but no longer
// matching the content (json-hash-mismatch) — a file edited outside the
// tool. nil means the file is trustworthy.
//
// Deliberately not part of loading a file: a read-only command (map, grep,
// search, diagram...) keeps working against a tampered file, because
// reading it does not launder the tampering the way writing over it would.
// Only something about to rewrite the file, and validate judging what is
// already on disk, call this.
//
// raw is the file exactly as it sits on disk, and value is what parsing it
// produced. The hash alone is not enough: parsing drops any key the type
// does not declare, so a field added by hand would vanish before the hash
// was recomputed and the file would pass. The binary is the only writer and
// always writes the same bytes for the same value, so the file must also be
// byte-for-byte what serializing value produces — anything else was written
// by something other than the binary.
func VerifyHash(path string, raw []byte, value HashCarrier) []finding.Finding {
	stored := value.StoredHash()
	if stored == "" {
		return []finding.Finding{finding.Errorf(finding.CodeJSONHashMissing,
			"%s has no hash: the field was removed outside the tool; restore the file (e.g. from git) rather than re-stamping it by hand", path).At(path)}
	}
	mismatch := []finding.Finding{finding.Errorf(finding.CodeJSONHashMismatch,
		"%s was changed outside the tool: it is no longer what the binary wrote; restore it (e.g. from git) rather than re-stamping it by hand", path).At(path)}
	want, err := StampHash(value)
	if err != nil {
		// The value already round-tripped through json.Unmarshal to reach
		// here, so a marshal of the same shape failing is not something a
		// caller can act on by reporting a hash problem.
		return nil
	}
	if want != stored {
		return mismatch
	}
	canonical, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil
	}
	if !bytes.Equal(raw, append(canonical, '\n')) {
		return mismatch
	}
	return nil
}
