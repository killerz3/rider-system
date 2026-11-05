# Rider dispatch demo

A small Go API registers riders through location updates, resolves pickup/drop coordinates to H3 cells, assigns an available nearby rider, and completes assignments. Redis holds live state and atomic Lua operations; SQLite stores durable write-through rider/assignment records.

## Run

```sh
docker compose up --build
```

The Docker daemon itself was not available in the build environment, so `docker compose up --build` was not run directly. As a substitute, the exact build command from the Dockerfile was reproduced by hand and the resulting binary was run through the full API flow against real Redis — all passed (see [VERIFICATION.md](VERIFICATION.md)). What's genuinely unverified is Docker/Compose-specific (inter-container DNS, the healthcheck gate, volume permissions) — please still run `docker compose up --build` yourself before relying on it. It enables Redis AOF and named volumes for Redis and SQLite.

For the local path used during verification, install Go 1.23.4 or compatible, GCC/CGO (for h3-go), Redis, and curl. The walkthrough also uses Python 3 to extract the assignment ID.

```sh
redis-server  # keep running in a separate terminal
go run .
# Or: go build ./... && ./rider-system
```

## Full flow

Use fresh rider/order IDs or a clean demo namespace for a new assignment. Creation returns 201; replay of the same order and coordinates returns 200. GET and completion return the assignment; health returns `{"status":"ok"}` when Redis responds.

```sh
curl -sS -X PUT localhost:8080/riders/r1/location -H 'Content-Type: application/json' -d '{"latitude":12.345,"longitude":45.678}'
curl -sS -X PUT localhost:8080/riders/r2/location -H 'Content-Type: application/json' -d '{"latitude":12.346,"longitude":45.678}'
assignment=$(curl -sS -X POST localhost:8080/assignments -H 'Content-Type: application/json' -d '{"order_id":"demo-order","pickup":{"latitude":12.345,"longitude":45.678},"drop":{"latitude":12.35,"longitude":45.68}}')
printf '%s\n' "$assignment"
id=$(printf '%s' "$assignment" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
curl -sS "localhost:8080/assignments/$id"
curl -sS -X POST "localhost:8080/assignments/$id/complete"
curl -sS localhost:8080/health
```

## Verification

With a real local Redis running:

```sh
go build ./...
go vet ./...
REDIS_ADDR=127.0.0.1:6379 go test ./... -v -race
```

These commands passed in the verification pass. Tests require an empty `dispatch:*` namespace in Redis database 15, refuse existing records, and remove their namespace afterward without FLUSHDB. See [VERIFICATION.md](VERIFICATION.md) for exact results and the compiled-server curl transcript.

## Configuration

| Environment variable | Default | Meaning |
|---|---|---|
| `REDIS_ADDR` | `localhost:6379` | Redis address; API uses database 0 |
| `HTTP_PORT` | `8080` | HTTP listening port |
| `H3_RESOLUTION` | `7` | Integer 0–15 |
| `GRID_DISTANCE` | `2` | Maximum H3 grid distance (rings) searched, integer 0–20 |
| `SEARCH_MODE` | `rounds` | `rounds` expands ring by ring; `disk` unions every cell within `GRID_DISTANCE` in one lookup |
| `SQLITE_PATH` | `riders.db` | SQLite file; tables created automatically |

## Design and limits

H3 maps locations to cells. Redis maps cells to rider IDs and each rider back to its cell, allowing atomic membership changes. Assignment lookup searches outward in rounds: round 0 is the pickup cell, round k the ring of cells exactly k steps away (`GridDiskDistances`), one `SUNION` per round, up to `GRID_DISTANCE`. Once a round finds an available rider, one more ring is read before claiming, because a rider just past that ring's edge can be closer to pickup than one at its far side. Candidates are ranked by haversine distance with rider ID as a tie break. If every candidate is claimed concurrently, the search continues from the next unread ring. Busy areas resolve in one or two cheap lookups; sparse areas pay a few extra Redis round trips. `SEARCH_MODE=disk` keeps the original single-union search over the whole disk. Lua checks availability and order idempotency atomically.

Candidate search is bounded to an H3 neighborhood and uses a snapshot; the one-ring lookahead is a heuristic, not an exact nearest guarantee across rings: it does not guarantee the globally nearest rider or recheck position at claim time. Redis is a single instance (no Cluster support). There is no authentication, location expiry, routing, ETA, or Redis rehydration from SQLite after data loss. Cell sets retain busy riders; availability is filtered separately. Keys and assignment history have no TTL. SQLite write ordering assumes one API process. Redis and SQLite have no shared transaction: a 503 can follow a successful Redis mutation. Health checks Redis only.

This demo was built with AI assistance: Claude coordinated Codex implementation and verification passes. Roughly 30 minutes of direct human involvement was the intended level of involvement, not a measured duration. The scope required an independent implementation, with an existing system allowed as broad conceptual inspiration only. See the [prompt history](docs/prompts.md) for the process and prompts (assembled separately by the coordinator).

Read the [offline project guide](docs/index.html) for the design discussion, API contracts, diagrams, and review path — also hosted at https://folio.kz3.dev/p/rider-dispatch.
