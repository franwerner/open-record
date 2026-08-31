// Package openrecord carries the files the binary ships with.
//
// The embed lives at the module root because go:embed cannot reach upwards: the
// catalogue and the skills are authored where they are read and reviewed, and
// this is the only place that can pick both up without a copy that would drift.
package openrecord

import "embed"

// Assets holds the concerns catalogue, which `level add` reads for a default
// description, and the agent skills, which `skills --emit` writes out.
//
//go:embed docs/concerns.md
//go:embed skills
var Assets embed.FS
