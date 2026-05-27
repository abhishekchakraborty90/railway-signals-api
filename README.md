# Railway Signals API

Read-only REST API (Go + [Echo](https://echo.labstack.com/)) over a dataset of
railway tracks and the signals along them. The source data is loaded into memory
once at startup — no external database.

## Architecture

```
main.go                     wiring, graceful shutdown
internal/
  config/constants.go       all ports, paths, defaults, query-param names
  models/models.go          raw (source) + domain (API) schemas
  store/store.go            in-memory DB: load JSON, normalize, index, query
  handlers/handlers.go      Echo routes (all GET)
crosstech-test-data.json    source dataset
```

Request flow: `main` loads + normalizes the JSON into `store`, then `handlers`
serve read-only queries against the indexed maps. No locking is needed — the
store is built once and only read.

## Run

```bash
go mod tidy        # first time: resolve deps
go run .
# PORT=9000 DATA_FILE=/path/to/data.json go run .
```

Listens on `:8080` by default.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/signals` | List canonical signals (paginated) |
| GET | `/signals/:id` | One signal |
| GET | `/signals/:id/tracks` | All tracks a signal appears on (relationship) |
| GET | `/tracks` | List tracks (paginated + filterable) |
| GET | `/tracks/:id` | One track |
| GET | `/tracks/:id/signals` | Signals on a track, with per-occurrence `elr`/`mileage` |

**Pagination** (`/signals`, `/tracks`): `?limit=` (default 50, max 500) and
`?offset=`. List responses are wrapped as `{ items, total, limit, offset }`.

**Track filters** (`/tracks`): `?track_id=` (exact), `?source=` / `?target=`
(case-insensitive substring), `?q=` (matches source OR target).

**Errors**: uniform JSON `{ "error": "...", "message": "..." }` with the
appropriate status (400 invalid param, 404 not found).

## Key design decisions

- **Data is quirky** — extracted via Python, so the same `signal_id` recurs
  across tracks with a *scrambled* `elr` (anagram noise, e.g. signal 453 shows
  `LPC5`/`PCL5`/`PLC5`) and a differing `mileage`. These are therefore treated
  as **occurrence-level** values, exposed only on `/tracks/:id/signals` — never
  as canonical signal attributes.
- **Normalized model** — a canonical `Signal` is just `{ signal_id, signal_name? }`,
  deduplicated by id. `signal_name` is nullable (106 signals are permanently
  unnamed) so it is omitted from JSON rather than emitted as `""`. Tracks
  reference signals by id only, avoiding duplication of signal data per track.
- **Load-once, fail-fast** — the dataset is parsed at startup. A missing/malformed
  *file* is fatal; individual malformed *records* (missing id/source/target,
  duplicate `track_id`) are skipped with a logged warning so one bad row cannot
  take down the API.
- **In-memory indexing** — maps for `track_id`, `signal_id`, track↔signal and
  signal↔track give O(1) lookups; sorted slices give deterministic list output.

## Notes

- Frontend (React/Vite/Tailwind/TanStack Query/Zod) from the brief is **not**
  included — backend only, per scope.
- The permissive CORS middleware lets a separately-hosted frontend call the API.
