package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/review"
	"github.com/franwerner/open-record/internal/store"
)

// readStoreFile reads a record by its store-relative path, for review.Open's
// discarded branch: it is never a snapshot, so the current file is read fresh
// on every call.
func readStoreFile(repo string) func(string) (string, error) {
	return func(path string) (string, error) {
		raw, err := os.ReadFile(filepath.Join(repo, store.Root, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
}

// reviewFindingFor maps one of review's sentinel errors to the coded failure
// a caller acts on. Anything review did not name explicitly — a read failure
// opening a discarded record, for instance — is reported as a usage failure
// with whatever it said.
func reviewFindingFor(err error, id, path string) error {
	switch {
	case errors.Is(err, review.ErrSearchNotFound):
		return Errorf(finding.CodeSearchNotFound, "no stored search %q", id)
	case errors.Is(err, review.ErrRecordNotInSearch):
		return Errorf(finding.CodeRecordNotInSearch, "%s is neither served nor discarded in search %q", path, id)
	case errors.Is(err, review.ErrRecordNotOpened):
		return Errorf(finding.CodeRecordNotOpened,
			"%s has not been opened in search %q; run `openrecord review open %s %s` first", path, id, id, path)
	default:
		return Errorf(finding.CodeUsage, "%v", err)
	}
}

func runReviewOpen(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("review open")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, path, err := twoArguments("review open", subject, "a search id and a record path")
	if err != nil {
		return err
	}

	search, err := review.Load(env.Repo, id)
	if err != nil {
		return reviewFindingFor(err, id, path)
	}
	entry, err := search.Open(path, readStoreFile(env.Repo), time.Now())
	if err != nil {
		return reviewFindingFor(err, id, path)
	}
	if err := review.Save(env.Repo, search); err != nil {
		return err
	}
	fmt.Fprint(env.Stdout, entry.Content)
	return nil
}

type reviewMarkOptions struct {
	verdict *string
}

func reviewMarkFlags(flags *flag.FlagSet) *reviewMarkOptions {
	return &reviewMarkOptions{
		verdict: flags.String("verdict", "", "governs, contradicts or unrelated"),
	}
}

func runReviewMark(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("review mark")
	options := reviewMarkFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, path, err := twoArguments("review mark", subject, "a search id and a record path")
	if err != nil {
		return err
	}
	verdict := strings.TrimSpace(*options.verdict)
	if verdict == "" {
		return Errorf(finding.CodeUsage, "review mark needs --verdict")
	}

	search, err := review.Load(env.Repo, id)
	if err != nil {
		return reviewFindingFor(err, id, path)
	}
	if err := search.Mark(path, verdict, time.Now()); err != nil {
		return reviewFindingFor(err, id, path)
	}
	if err := review.Save(env.Repo, search); err != nil {
		return err
	}
	return env.WriteJSON(map[string]any{
		"id":           id,
		"path":         path,
		"verdict":      verdict,
		"completed_at": search.CompletedAt,
	})
}

// reviewStatusEntry is what review status reports for one record: whether it
// was opened, and its verdict if it has one.
type reviewStatusEntry struct {
	Opened  bool   `json:"opened"`
	Verdict string `json:"verdict,omitempty"`
}

type reviewStatusReport struct {
	ID          string                       `json:"id"`
	Served      map[string]reviewStatusEntry `json:"served"`
	Discarded   map[string]reviewStatusEntry `json:"discarded,omitempty"`
	CompletedAt *time.Time                   `json:"completed_at"`
}

// runReviewStatus never mutates: it reads the stored search and reports what
// is there, exiting non-zero only while a served record still lacks a
// verdict.
func runReviewStatus(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("review status")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("review status", subject, "a search id")
	if err != nil {
		return err
	}

	search, err := review.Load(env.Repo, id)
	if err != nil {
		return reviewFindingFor(err, id, "")
	}

	served := map[string]reviewStatusEntry{}
	pending := false
	for _, entry := range search.Served {
		served[entry.Path] = reviewStatusEntry{Opened: entry.OpenedAt != nil, Verdict: entry.Verdict}
		if entry.Verdict == "" {
			pending = true
		}
	}
	discarded := map[string]reviewStatusEntry{}
	for _, entry := range search.Discarded {
		if entry.OpenedAt == nil && entry.Verdict == "" {
			continue
		}
		discarded[entry.Path] = reviewStatusEntry{Opened: entry.OpenedAt != nil, Verdict: entry.Verdict}
	}

	report := reviewStatusReport{ID: search.ID, Served: served, Discarded: discarded, CompletedAt: search.CompletedAt}
	if err := env.WriteJSON(report); err != nil {
		return err
	}
	if pending {
		return errExitWithReport
	}
	return nil
}
