// Package models defines the typed schemas for the railway domain.
//
// Two layers exist on purpose:
//
//   - "raw" types mirror the quirky source JSON exactly (extracted via Python,
//     so they carry inconsistencies — see store normalization).
//   - "domain" types are the clean, normalized shapes the API returns.
//
// Quirk handled here: the same signal_id recurs across many tracks, but its
// `elr` is scrambled (anagram noise) and `mileage` differs per occurrence.
// Therefore elr/mileage are NOT canonical signal attributes — they are
// occurrence-level data attached only to the track↔signal relationship.
package models

// ---------- Raw source schema (matches the JSON file verbatim) ----------

// RawTrack is one record in the source array.
type RawTrack struct {
	TrackID   int             `json:"track_id"`
	Source    string          `json:"source"`
	Target    string          `json:"target"`
	SignalIDs []RawSignalOccur `json:"signal_ids"`
}

// RawSignalOccur is one signal as it appears embedded inside a track.
// signal_name and mileage are nullable in the source data.
type RawSignalOccur struct {
	SignalID   int      `json:"signal_id"`
	SignalName *string  `json:"signal_name"`
	ELR        string   `json:"elr"`
	Mileage    *float64 `json:"mileage"`
}

// ---------- Domain schema (clean, normalized API output) ----------

// Signal is a canonical, deduplicated signal.
// Name is a pointer so the 106 permanently-nameless signals serialize as a
// clean object without a name field rather than as an empty string.
type Signal struct {
	ID   int     `json:"signal_id"`
	Name *string `json:"signal_name,omitempty"`
}

// Track is a section of line between two locations, carrying a set of signals.
// SignalIDs references signals by id only (the full signal set lives in the
// signal collection — avoids duplicating signal data on every track).
type Track struct {
	TrackID   int    `json:"track_id"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	SignalIDs []int  `json:"signal_ids"`
}

// TrackSignal is the relationship row: a signal as it appears ON a specific
// track, preserving the occurrence-level elr/mileage. This is the only place
// elr/mileage are exposed, because they are unreliable per-occurrence values.
type TrackSignal struct {
	SignalID int      `json:"signal_id"`
	Name     *string  `json:"signal_name,omitempty"`
	ELR      string   `json:"elr"`
	Mileage  *float64 `json:"mileage"`
}

// ---------- Response envelopes ----------

// Page wraps a list response with pagination metadata.
type Page[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`  // total matching items before paging
	Limit  int `json:"limit"`  // page size applied
	Offset int `json:"offset"` // offset applied
}

// ErrorResponse is the uniform error body returned for all 4xx/5xx.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
