// Package config centralizes all tunable values for the API.
// Per project convention, no magic numbers/strings live in business logic —
// every threshold, default, and route fragment is declared here.
package config

import "time"

// Server.
const (
	// DefaultPort is used when the PORT env var is unset.
	DefaultPort = "8080"
	// EnvPort is the environment variable name overriding the listen port.
	EnvPort = "PORT"
	// EnvDataFile overrides the path to the source JSON data file.
	EnvDataFile = "DATA_FILE"
	// DefaultDataFile is the source dataset loaded into memory at startup.
	DefaultDataFile = "crosstech-test-data.json"

	// ShutdownTimeout bounds graceful shutdown.
	ShutdownTimeout = 10 * time.Second
)

// Pagination. Applied to the list endpoints (/signals, /tracks).
const (
	// QueryLimit / QueryOffset are the pagination query-param names.
	QueryLimit  = "limit"
	QueryOffset = "offset"

	// DefaultLimit is the page size when ?limit= is absent.
	DefaultLimit = 50
	// MaxLimit caps an explicit ?limit= to protect the server.
	MaxLimit = 500
	// MinOffset is the smallest accepted offset.
	MinOffset = 0
)

// Track filter query-param names.
const (
	// QueryTrackID filters tracks by exact track_id.
	QueryTrackID = "track_id"
	// QuerySource / QueryTarget filter by case-insensitive substring.
	QuerySource = "source"
	QueryTarget = "target"
	// QueryText matches either source OR target (case-insensitive substring).
	QueryText = "q"
)

// Path params.
const (
	ParamSignalID = "id"
	ParamTrackID  = "id"
)
