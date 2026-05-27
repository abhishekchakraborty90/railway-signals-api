// Package store is the in-memory database. It loads the source JSON once at
// startup, normalizes the quirky data into clean domain entities, and serves
// read-only lookups. No external database is used (per the brief).
package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/crosstech/railway-signals-api/internal/models"
)

// pythonNonFinite matches the non-standard numeric literals that Python's
// json.dumps emits for non-finite floats (NaN, Infinity, -Infinity). These are
// invalid per the JSON spec, so Go's encoding/json rejects them. The dataset
// uses `"mileage": NaN` for unknown mileages, so we rewrite these tokens to
// `null` before parsing (mileage is already a nullable *float64). Verified that
// no genuine string value in the dataset contains these tokens.
var pythonNonFinite = regexp.MustCompile(`-?\bInfinity\b|\bNaN\b`)

// Store holds normalized, indexed railway data. It is built once and then
// only read, so it needs no locking.
type Store struct {
	tracks      []models.Track             // sorted by track_id
	tracksByID  map[int]models.Track       // track_id -> track
	signals     []models.Signal            // sorted by signal_id, deduplicated
	signalsByID map[int]models.Signal      // signal_id -> canonical signal
	// occurrences[trackID] = signals as they appear on that track (with elr/mileage)
	occurrences map[int][]models.TrackSignal
	// tracksBySignal[signalID] = track_ids the signal appears on
	tracksBySignal map[int][]int
}

// Load reads and normalizes the dataset at path. It fails fast: a missing or
// malformed file is fatal, because the API is useless without data. Individual
// records that are structurally invalid are skipped with a warning rather than
// aborting the whole load (the brief calls for handling data inconsistencies).
func Load(path string) (*Store, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read data file %q: %w", path, err)
	}

	// Repair Python's non-standard JSON (NaN/Infinity) into valid `null`.
	bytes = pythonNonFinite.ReplaceAll(bytes, []byte("null"))

	var raw []models.RawTrack
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return nil, fmt.Errorf("parse data file %q: %w", path, err)
	}

	s := &Store{
		tracksByID:     make(map[int]models.Track),
		signalsByID:    make(map[int]models.Signal),
		occurrences:    make(map[int][]models.TrackSignal),
		tracksBySignal: make(map[int][]int),
	}
	s.normalize(raw)
	return s, nil
}

// normalize folds the raw records into the indexed domain model.
//
// Normalization rules (derived from analysis of the data):
//   - A signal is canonical by signal_id; its name is the first non-null name
//     seen (names never conflict across occurrences in the dataset).
//   - elr/mileage are occurrence-level and kept only on the relationship.
//   - Duplicate track_ids and signals already-seen on a track are de-duped.
func (s *Store) normalize(raw []models.RawTrack) {
	for i, rt := range raw {
		// Skip structurally invalid track records, but keep going.
		if rt.TrackID == 0 || rt.Source == "" || rt.Target == "" {
			log.Printf("warning: skipping malformed track at index %d (id=%d source=%q target=%q)",
				i, rt.TrackID, rt.Source, rt.Target)
			continue
		}
		if _, dup := s.tracksByID[rt.TrackID]; dup {
			log.Printf("warning: duplicate track_id %d at index %d — keeping first occurrence", rt.TrackID, i)
			continue
		}

		track := models.Track{TrackID: rt.TrackID, Source: rt.Source, Target: rt.Target}
		seenOnTrack := make(map[int]struct{}, len(rt.SignalIDs))

		for _, occ := range rt.SignalIDs {
			s.upsertSignal(occ)

			// Each signal counts once per track for the id list / relationship.
			if _, seen := seenOnTrack[occ.SignalID]; !seen {
				seenOnTrack[occ.SignalID] = struct{}{}
				track.SignalIDs = append(track.SignalIDs, occ.SignalID)
				s.tracksBySignal[occ.SignalID] = append(s.tracksBySignal[occ.SignalID], rt.TrackID)
			}

			// Relationship row preserves the raw occurrence values verbatim,
			// but resolves the name to the canonical one for convenience.
			s.occurrences[rt.TrackID] = append(s.occurrences[rt.TrackID], models.TrackSignal{
				SignalID: occ.SignalID,
				Name:     s.signalsByID[occ.SignalID].Name,
				ELR:      occ.ELR,
				Mileage:  occ.Mileage,
			})
		}

		sort.Ints(track.SignalIDs)
		s.tracksByID[rt.TrackID] = track
	}

	s.buildSortedSlices()
}

// upsertSignal records a canonical signal, filling in a name the first time a
// non-null one is seen for that id.
func (s *Store) upsertSignal(occ models.RawSignalOccur) {
	existing, ok := s.signalsByID[occ.SignalID]
	if !ok {
		s.signalsByID[occ.SignalID] = models.Signal{ID: occ.SignalID, Name: occ.SignalName}
		return
	}
	if existing.Name == nil && occ.SignalName != nil {
		existing.Name = occ.SignalName
		s.signalsByID[occ.SignalID] = existing
	}
}

// buildSortedSlices materializes deterministic, sorted slices for list output.
func (s *Store) buildSortedSlices() {
	s.tracks = make([]models.Track, 0, len(s.tracksByID))
	for _, t := range s.tracksByID {
		s.tracks = append(s.tracks, t)
	}
	sort.Slice(s.tracks, func(i, j int) bool { return s.tracks[i].TrackID < s.tracks[j].TrackID })

	s.signals = make([]models.Signal, 0, len(s.signalsByID))
	for _, sig := range s.signalsByID {
		s.signals = append(s.signals, sig)
	}
	sort.Slice(s.signals, func(i, j int) bool { return s.signals[i].ID < s.signals[j].ID })

	// Keep the per-signal track lists sorted and unique for stable output.
	for id, ids := range s.tracksBySignal {
		sort.Ints(ids)
		s.tracksBySignal[id] = ids
	}
}

// ---------- Read API ----------

// Signals returns all canonical signals (sorted by id).
func (s *Store) Signals() []models.Signal { return s.signals }

// SignalByID returns the canonical signal and whether it exists.
func (s *Store) SignalByID(id int) (models.Signal, bool) {
	sig, ok := s.signalsByID[id]
	return sig, ok
}

// Tracks returns all tracks (sorted by track_id).
func (s *Store) Tracks() []models.Track { return s.tracks }

// TrackByID returns the track and whether it exists.
func (s *Store) TrackByID(id int) (models.Track, bool) {
	t, ok := s.tracksByID[id]
	return t, ok
}

// SignalsOnTrack returns the occurrence-level signals (with elr/mileage) for a
// track, or false if the track does not exist.
func (s *Store) SignalsOnTrack(trackID int) ([]models.TrackSignal, bool) {
	if _, ok := s.tracksByID[trackID]; !ok {
		return nil, false
	}
	return s.occurrences[trackID], true
}

// TracksForSignal returns every track a signal appears on (the bonus
// relationship endpoint), or false if the signal does not exist.
func (s *Store) TracksForSignal(signalID int) ([]models.Track, bool) {
	if _, ok := s.signalsByID[signalID]; !ok {
		return nil, false
	}
	ids := s.tracksBySignal[signalID]
	out := make([]models.Track, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.tracksByID[id])
	}
	return out, true
}

// FilterTracks applies the optional filters. A zero-value filter is a no-op.
// trackID == nil means "any"; source/target/text are case-insensitive substrings.
func (s *Store) FilterTracks(trackID *int, source, target, text string) []models.Track {
	source, target, text = strings.ToLower(source), strings.ToLower(target), strings.ToLower(text)
	out := make([]models.Track, 0, len(s.tracks))
	for _, t := range s.tracks {
		if trackID != nil && t.TrackID != *trackID {
			continue
		}
		if source != "" && !strings.Contains(strings.ToLower(t.Source), source) {
			continue
		}
		if target != "" && !strings.Contains(strings.ToLower(t.Target), target) {
			continue
		}
		if text != "" &&
			!strings.Contains(strings.ToLower(t.Source), text) &&
			!strings.Contains(strings.ToLower(t.Target), text) {
			continue
		}
		out = append(out, t)
	}
	return out
}
