// Package review is the lifecycle a stored search goes through after it is
// made: served records get opened and given a verdict, until every one of
// them has one and the search is complete.
//
// A stored search lives under .openrecord/.searches, one JSON file per id,
// written through a temporary file and renamed so an interrupted write never
// leaves a half-written search behind. The directory is created with its own
// .gitignore, so nothing here ever needs to touch a project's own one.
package review

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/franwerner/open-record/internal/store"
)

// Verdicts. contradicts implies governs — a record that contradicts the
// context is, by definition, one that governs it.
const (
	VerdictGoverns     = "governs"
	VerdictContradicts = "contradicts"
	VerdictUnrelated   = "unrelated"
)

func validVerdict(v string) bool {
	switch v {
	case VerdictGoverns, VerdictContradicts, VerdictUnrelated:
		return true
	default:
		return false
	}
}

// Errors Open and Mark return. A caller maps these to a coded failure with
// errors.Is — they are never shown to a person as-is.
var (
	// ErrSearchNotFound means the id names no stored search.
	ErrSearchNotFound = errors.New("search not found")
	// ErrRecordNotInSearch means the path is neither served nor discarded in
	// this search.
	ErrRecordNotInSearch = errors.New("record is neither served nor discarded in this search")
	// ErrRecordNotOpened means Mark was called before Open ever was, for this
	// record in this search.
	ErrRecordNotOpened = errors.New("record has not been opened")
	// ErrInvalidVerdict means the verdict is outside the three the lifecycle
	// knows.
	ErrInvalidVerdict = errors.New("invalid verdict")
)

// searchesDirName is the directory every stored search lives under, inside
// the store root.
const searchesDirName = ".searches"

// Version is the one stored-search shape this build knows how to read and
// write.
const Version = 1

// Literal is the local, exact half of a record's hit — never sent to Jev,
// only recorded alongside the score so a reviewer sees why a record matched.
type Literal struct {
	Terms []string `json:"terms"`
	Line  int      `json:"line"`
	Text  string   `json:"text"`
	Hits  int      `json:"hits"`
}

// Semantic is qmd's half of a record's hit.
type Semantic struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Entry is one record inside a stored search. A served entry carries its
// full content, snapshotted at search time. A discarded entry starts with no
// content at all — Path only — until it is opened, at which point the
// current file is read and stored here.
type Entry struct {
	Path     string     `json:"path"`
	Content  string     `json:"content,omitempty"`
	Literal  *Literal   `json:"literal,omitempty"`
	Semantic *Semantic  `json:"semantic,omitempty"`
	OpenedAt *time.Time `json:"opened_at,omitempty"`
	Verdict  string     `json:"verdict,omitempty"`
}

// Inputs is exactly what the caller gave `search`, kept so a stored search is
// self-describing.
type Inputs struct {
	For      []string `json:"for"`
	Literal  []string `json:"literal"`
	Semantic string   `json:"semantic"`
	Context  string   `json:"context"`
	Omit     []string `json:"omit,omitempty"`
}

// Search is one stored search: its inputs, what it served and discarded, and
// the review state layered onto the served (and later, opened discarded)
// entries.
type Search struct {
	Version     int        `json:"version"`
	ID          string     `json:"id"`
	Model       string     `json:"model"`
	Inputs      Inputs     `json:"inputs"`
	Served      []Entry    `json:"served"`
	Discarded   []Entry    `json:"discarded"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// New builds a search with its creation-time invariants already applied:
// UpdatedAt starts equal to CreatedAt, and a search that serves nothing is
// complete from the moment it exists — there is nothing left to review.
func New(id, model string, inputs Inputs, served, discarded []Entry, now time.Time) *Search {
	s := &Search{
		Version:   Version,
		ID:        id,
		Model:     model,
		Inputs:    inputs,
		Served:    served,
		Discarded: discarded,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if len(served) == 0 {
		completed := now
		s.CompletedAt = &completed
	}
	return s
}

// NewID names a search: a sortable, readable UTC timestamp, plus 6 hex
// characters from crypto/rand so two searches made in the same second never
// collide.
func NewID(now time.Time) string {
	stamp := now.UTC().Format("20060102T150405Z")
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand failing is not a condition this package can recover
		// from meaningfully, but an id still has to exist — nanoseconds are
		// not cryptographically random, only enough to avoid a collision
		// with whatever else is running right now.
		return fmt.Sprintf("%s-%06x", stamp, time.Now().UnixNano()&0xffffff)
	}
	return stamp + "-" + hex.EncodeToString(buf[:])
}

func searchesDir(repo string) string {
	return filepath.Join(repo, store.Root, searchesDirName)
}

// Save writes a search through a temporary file and renames it into place,
// so an interrupted write leaves no half-written search behind. The
// directory is created, with its own .gitignore, the first time anything is
// saved.
func Save(repo string, s *Search) error {
	dir := searchesDir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := ensureGitignore(dir); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(dir, s.ID+".json")
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, target)
}

// ensureGitignore writes the directory's own .gitignore the first time it is
// needed. It never touches a project's own .gitignore — this is the one
// thing that lets every user's store stay untouched by this feature.
func ensureGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("*\n"), 0o644)
}

// Load reads a stored search by id. An unknown or malformed id, or one that
// would escape the searches directory, is reported identically as
// ErrSearchNotFound — there is nothing more specific a caller could act on.
func Load(repo, id string) (*Search, error) {
	safeID, err := sanitizeID(id)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(searchesDir(repo), safeID+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrSearchNotFound, id)
		}
		return nil, err
	}
	var s Search
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSearchNotFound, id)
	}
	return &s, nil
}

// sanitizeID refuses anything that could climb out of the searches
// directory. An id is never a path — it is a value this package minted
// itself — so anything shaped like one is wrong rather than merely unusual.
func sanitizeID(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return "", fmt.Errorf("%w: %q", ErrSearchNotFound, id)
	}
	return id, nil
}

func (s *Search) findServed(path string) *Entry {
	for index := range s.Served {
		if s.Served[index].Path == path {
			return &s.Served[index]
		}
	}
	return nil
}

func (s *Search) findDiscarded(path string) *Entry {
	for index := range s.Discarded {
		if s.Discarded[index].Path == path {
			return &s.Discarded[index]
		}
	}
	return nil
}

// Open prints and marks one record opened, advancing UpdatedAt. A served
// record returns the snapshot stored at search time. A discarded record is
// read fresh through read on every call — it was never snapshotted — and
// stays discarded: opening it never moves it into Served.
func (s *Search) Open(path string, read func(string) (string, error), now time.Time) (Entry, error) {
	if entry := s.findServed(path); entry != nil {
		opened := now
		entry.OpenedAt = &opened
		s.UpdatedAt = now
		return *entry, nil
	}
	if entry := s.findDiscarded(path); entry != nil {
		content, err := read(path)
		if err != nil {
			return Entry{}, err
		}
		entry.Content = content
		opened := now
		entry.OpenedAt = &opened
		s.UpdatedAt = now
		return *entry, nil
	}
	return Entry{}, fmt.Errorf("%w: %s", ErrRecordNotInSearch, path)
}

// Mark stores a verdict for a record that was opened earlier — served or
// discarded — and advances UpdatedAt. Marking again replaces the earlier
// verdict. Only a served record's verdict can complete the search: the mark
// that gives the last served record its verdict sets CompletedAt, and
// nothing clears it afterward. Marking a discarded record never affects
// completion.
func (s *Search) Mark(path, verdict string, now time.Time) error {
	if !validVerdict(verdict) {
		return fmt.Errorf("%w: %q", ErrInvalidVerdict, verdict)
	}
	servedEntry := s.findServed(path)
	entry := servedEntry
	if entry == nil {
		entry = s.findDiscarded(path)
	}
	if entry == nil {
		return fmt.Errorf("%w: %s", ErrRecordNotInSearch, path)
	}
	if entry.OpenedAt == nil {
		return fmt.Errorf("%w: %s", ErrRecordNotOpened, path)
	}

	entry.Verdict = verdict
	s.UpdatedAt = now

	if servedEntry != nil {
		s.maybeComplete(now)
	}
	return nil
}

// maybeComplete sets CompletedAt the first time every served record has a
// verdict. Once set, nothing in this package clears it — a later remark of a
// served record changes its verdict, never the search's completion.
func (s *Search) maybeComplete(now time.Time) {
	if s.CompletedAt != nil {
		return
	}
	for _, entry := range s.Served {
		if entry.Verdict == "" {
			return
		}
	}
	completed := now
	s.CompletedAt = &completed
}
