# Implementation handoff (internal)

## Redis model
All keys use `dispatch:`. No keys have a TTL.
- `dispatch:cell:{h3_cell}`: SET of rider IDs, including unavailable riders. Empty sets naturally disappear.
- `dispatch:rider:{rider_id}`: HASH: `id`, `latitude`, `longitude`, `h3_cell`, `available` (`1`/`0`), `current_assignment_id` (empty string when unassigned), `updated_at`.
- `dispatch:assignment:{assignment_id}`: HASH: `record` (JSON immutable assignment fields plus initial status), `payload` (canonical JSON pickup/drop coordinates for idempotency), `status` (`active`/`completed`), `rider_id`, `completed_at` (empty until completed). Reads overlay status/completed_at onto record.
- `dispatch:order:{order_id}`: STRING assignment ID.
- `dispatch:no-candidate:{random_assignment_id}` is only a nonexistent sentinel key read by the final claim attempt; never written. It allows the atomic order check even when no candidate remains.
No expiry means stale locations remain eligible indefinitely; freshness filtering and cleanup are future considerations. Rider movement is one Lua operation preserving assignment state. Claims and completion are also Lua operations. Single Redis instance only.

## Dependencies/build
`github.com/uber/h3-go/v4 v4.3.0`: required v4 library, maintained Go interface to vendored H3 C, no system libh3 required. CGO enabled. `go build ./...` and `go vet ./...` passed with this CGO library. Redis client v9.7.0; modernc.org/sqlite v1.34.5 (pure Go).
Docker uses golang:1.23.4-bookworm (includes GCC) and debian:bookworm-slim (matching glibc runtime); no Docker execution was possible here. Compose persists both SQLite and Redis; Redis AOF enabled.

## Decisions where scope was open
- Flat main package; Go standard ServeMux method/path routing. IDs are opaque nonempty path segments/order strings; assignment IDs are random 128-bit lowercase hex.
- Defaults: REDIS_ADDR=localhost:6379, HTTP_PORT=8080, H3_RESOLUTION=7 (single named default constant), GRID_DISTANCE=2, SQLITE_PATH=riders.db. H3 resolution validated 0..15; grid distance bounded 0..20 to avoid accidental enormous searches.
- Candidate union uses Redis SUNION, reads rider hashes, filters availability, sorts by haversine meters (Earth radius 6,371,000m), lexical rider ID tie break. Snapshot and bounded grid search, not globally nearest; motion between snapshot/claim accepted. Drop only recorded.
- Assignment creation returns 201, replay 200; no candidate returns 409 with code `no_rider_available`; conflicting order coordinates returns 409 `order_payload_conflict`. Successful location/complete/get return 200. Missing assignment 404. Health returns JSON status ok/degraded and 200/503 based only on Redis PING.
- Coordinate payload equality is numeric after decoding float64, normalizing negative zero. JSON formatting/key order is irrelevant; equivalent geographic coordinates with different numeric longitude (180 versus -180) remain distinct payloads. Unknown JSON fields, missing coordinates, trailing JSON, and bodies over 1MiB rejected. Errors have code/message. Unknown routes or unsupported methods return JSON 404 `route_not_found`.
- JSON uses latitude/longitude, h3_cell, current_assignment_id (null if absent); assignments use nested pickup/drop plus pickup_h3_cell/drop_h3_cell. Timestamps are UTC RFC3339Nano strings, completed_at null until complete; status is active/completed.
- All Redis calls use a request-derived 3-second context, with client socket/dial timeouts and context deadlines enabled. The same deadline covers the full mutation/read/write-through sequence. Server read-header/read/write/idle timeouts are 5/10/15/60 seconds. Graceful shutdown budget 5 seconds.
- SQLite WAL, busy timeout 3 seconds, one connection; automatic CREATE TABLE IF NOT EXISTS. Each successful mutation writes the resulting rider and assignment (when applicable) in one SQL transaction. Assignment retry and repeated completion also write through, allowing repair after a prior SQL failure. Location retries rewrite the current rider. API never reads SQLite.
- Process-local mutex covers mutations and synchronous write-through, preventing local concurrent handlers from writing older snapshots after newer ones. Redis Lua remains the only atomic claim authority, including across service instances; no SQL lookup or claim logic. This demo assumes one API process for SQL write ordering. Multi-instance coordinated SQL snapshot ordering is future work.
- SQL failure after Redis success returns 503 `storage_unavailable`, with a message that Redis may already have changed. No distributed transaction/rollback exists between Redis and SQLite, so forward durability is guaranteed only for successful responses. Redis outages also return 503. The system cannot safely roll back a claim on a SQL error.
- SQLite is write-only for the live API. Rehydrating Redis from SQLite on restart is NOT implemented; future consideration. A restart with retained Redis data works; losing Redis data leaves durable history in SQLite but requires a future restore procedure. Assignment history/order mappings are retained without expiry.

## Deviations/limitations
No intentional functional scope deviations. No tests were written per pass instructions. Existing empty docs directory was left untouched; no README or docs were written. Build outputs ignored. No commits made.
