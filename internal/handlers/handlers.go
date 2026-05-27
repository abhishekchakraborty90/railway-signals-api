// Package handlers wires HTTP routes to the in-memory store. All endpoints are
// read-only (GET) per the brief. Errors are returned as a uniform JSON body.
package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/crosstech/railway-signals-api/internal/config"
	"github.com/crosstech/railway-signals-api/internal/models"
	"github.com/crosstech/railway-signals-api/internal/store"
)

// Handler holds dependencies for the route handlers.
type Handler struct {
	store *store.Store
}

// New creates a Handler backed by the given store.
func New(s *store.Store) *Handler { return &Handler{store: s} }

// Register attaches all routes to the Echo instance.
func (h *Handler) Register(e *echo.Echo) {
	e.GET("/health", h.Health)

	e.GET("/signals", h.ListSignals)
	e.GET("/signals/:id", h.GetSignal)
	e.GET("/signals/:id/tracks", h.GetTracksForSignal) // bonus relationship

	e.GET("/tracks", h.ListTracks)
	e.GET("/tracks/:id", h.GetTrack)
	e.GET("/tracks/:id/signals", h.GetSignalsOnTrack)
}

// Health is a liveness probe.
func (h *Handler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ListSignals returns a paginated list of canonical signals.
func (h *Handler) ListSignals(c echo.Context) error {
	limit, offset, err := paginate(c)
	if err != nil {
		return err
	}
	all := h.store.Signals()
	items := pageSlice(all, limit, offset)
	return c.JSON(http.StatusOK, models.Page[models.Signal]{
		Items: items, Total: len(all), Limit: limit, Offset: offset,
	})
}

// GetSignal returns one canonical signal by id.
func (h *Handler) GetSignal(c echo.Context) error {
	id, err := intParam(c, config.ParamSignalID)
	if err != nil {
		return err
	}
	sig, ok := h.store.SignalByID(id)
	if !ok {
		return notFound("signal", id)
	}
	return c.JSON(http.StatusOK, sig)
}

// GetTracksForSignal returns every track a signal appears on (bonus endpoint).
func (h *Handler) GetTracksForSignal(c echo.Context) error {
	id, err := intParam(c, config.ParamSignalID)
	if err != nil {
		return err
	}
	tracks, ok := h.store.TracksForSignal(id)
	if !ok {
		return notFound("signal", id)
	}
	return c.JSON(http.StatusOK, tracks)
}

// ListTracks returns a paginated, optionally filtered list of tracks.
// Filters: ?track_id= (exact), ?source= / ?target= (ci substring), ?q= (either).
func (h *Handler) ListTracks(c echo.Context) error {
	limit, offset, err := paginate(c)
	if err != nil {
		return err
	}

	var trackID *int
	if raw := c.QueryParam(config.QueryTrackID); raw != "" {
		v, convErr := strconv.Atoi(raw)
		if convErr != nil {
			return badRequest("invalid track_id", "track_id must be an integer")
		}
		trackID = &v
	}

	all := h.store.FilterTracks(
		trackID,
		c.QueryParam(config.QuerySource),
		c.QueryParam(config.QueryTarget),
		c.QueryParam(config.QueryText),
	)
	items := pageSlice(all, limit, offset)
	return c.JSON(http.StatusOK, models.Page[models.Track]{
		Items: items, Total: len(all), Limit: limit, Offset: offset,
	})
}

// GetTrack returns one track by id.
func (h *Handler) GetTrack(c echo.Context) error {
	id, err := intParam(c, config.ParamTrackID)
	if err != nil {
		return err
	}
	t, ok := h.store.TrackByID(id)
	if !ok {
		return notFound("track", id)
	}
	return c.JSON(http.StatusOK, t)
}

// GetSignalsOnTrack returns the occurrence-level signals (with elr/mileage) on
// a track. This is the only endpoint exposing elr/mileage, as those values are
// occurrence-specific and unreliable as canonical signal attributes.
func (h *Handler) GetSignalsOnTrack(c echo.Context) error {
	id, err := intParam(c, config.ParamTrackID)
	if err != nil {
		return err
	}
	sigs, ok := h.store.SignalsOnTrack(id)
	if !ok {
		return notFound("track", id)
	}
	return c.JSON(http.StatusOK, sigs)
}

// ---------- helpers ----------

// intParam parses an integer path parameter, returning a 400 on failure.
func intParam(c echo.Context, name string) (int, error) {
	v, err := strconv.Atoi(c.Param(name))
	if err != nil {
		return 0, badRequest("invalid "+name, name+" must be an integer")
	}
	return v, nil
}

// paginate reads and validates ?limit= and ?offset=, applying defaults/caps.
func paginate(c echo.Context) (limit, offset int, err error) {
	limit = config.DefaultLimit
	if raw := c.QueryParam(config.QueryLimit); raw != "" {
		v, convErr := strconv.Atoi(raw)
		if convErr != nil || v < 1 {
			return 0, 0, badRequest("invalid limit", "limit must be a positive integer")
		}
		if v > config.MaxLimit {
			v = config.MaxLimit
		}
		limit = v
	}
	if raw := c.QueryParam(config.QueryOffset); raw != "" {
		v, convErr := strconv.Atoi(raw)
		if convErr != nil || v < config.MinOffset {
			return 0, 0, badRequest("invalid offset", "offset must be a non-negative integer")
		}
		offset = v
	}
	return limit, offset, nil
}

// pageSlice returns the [offset, offset+limit) window of items, never panicking
// on out-of-range bounds. Returns a non-nil empty slice past the end.
func pageSlice[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

// notFound / badRequest return a non-nil *echo.HTTPError whose Message is an
// ErrorResponse. They do NOT write the response themselves — the centralized
// HTTPErrorHandler (see ErrorHandler) renders it. Returning a real error is
// what makes the caller's `if err != nil { return err }` actually short-circuit.
func notFound(kind string, id int) error {
	return echo.NewHTTPError(http.StatusNotFound, models.ErrorResponse{
		Error:   "not_found",
		Message: kind + " " + strconv.Itoa(id) + " not found",
	})
}

func badRequest(errCode, msg string) error {
	return echo.NewHTTPError(http.StatusBadRequest, models.ErrorResponse{Error: errCode, Message: msg})
}

// ErrorHandler is the single place every error becomes a uniform JSON body.
// It unwraps *echo.HTTPError, rendering our ErrorResponse when present and
// falling back to a generic message otherwise (e.g. 404 for unknown routes).
func ErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	code := http.StatusInternalServerError
	body := models.ErrorResponse{Error: "internal_error", Message: "unexpected error"}

	var he *echo.HTTPError
	if errors.As(err, &he) {
		code = he.Code
		if er, ok := he.Message.(models.ErrorResponse); ok {
			body = er
		} else {
			body = models.ErrorResponse{
				Error:   strings.ReplaceAll(strings.ToLower(http.StatusText(code)), " ", "_"),
				Message: fmt.Sprint(he.Message),
			}
		}
	}

	if jsonErr := c.JSON(code, body); jsonErr != nil {
		c.Logger().Error(jsonErr)
	}
}
