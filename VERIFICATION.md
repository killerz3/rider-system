# Verification

Reviewed NOTES-impl.md and all six implementation Go files before writing tests. No implementation bug was found; no production code was changed.

## Commands

```sh
gofmt -w verification_test.go
go build ./...
go vet ./...
REDIS_ADDR=127.0.0.1:6379 go test ./... -v -race
mkdir -p .verification-smoke
python3 .verification-smoke/run.py
```

The smoke runner selected a free port, launched the compiled `./rider-system`, executed the exact curl commands below, and terminated the server. Tests use live Redis at 127.0.0.1:6379, database 15. They refuse an existing `dispatch:*` namespace and delete only that test namespace afterward; they never FLUSHDB. SQLite test databases are created and removed inside this workspace. The smoke server uses default Redis database 0 and a separate SQLite file inside the workspace.

## Scenario results

| Scenario | Expected | Method | Actual result |
|---|---|---|---|
| Rider creation | Available rider, no assignment, correct H3 | HTTP location update and H3 library comparison | PASS |
| Same-cell update | Cell unchanged | Update to cell center | PASS |
| Cross-cell update | New membership present, old absent | Update to neighbor center; Redis SISMEMBER checks | PASS |
| Assigned rider moves | Availability and assignment preserved | Location update after claim; Redis state | PASS |
| Pickup/drop H3 | Correct cells and coordinates | Compare independent H3 conversions and returned points | PASS |
| Neighboring rider | Claim succeeds outside exact pickup cell | Sole rider placed in adjacent H3 cell | PASS |
| No rider available | 409 no_rider_available | Empty namespace, far rider, busy rider cases | PASS |
| Concurrent distinct orders | One successful claim per rider | 24 goroutines, 3 riders, independent server mutexes, real Redis Lua | PASS; 3 unique assignments |
| Concurrent identical orders | Same assignment for every caller | 24 goroutines, 1 rider, independent server mutexes | PASS; 1 created assignment, 24 successes |
| Changed pickup or drop | 409 order_payload_conflict | Two table-driven retries | PASS |
| Completion | Rider available, assignment completed | HTTP completion, Redis reads | PASS |
| Repeated completion | Same completion timestamp | Repeat completion, compare response | PASS |
| Old completion after new claim | New claim remains unavailable and unchanged | Complete A, claim B, repeat completion A; Redis checks | PASS |
| Invalid JSON/coordinates/fields | Clear 400 code/message | Table-driven missing/null/type/range/malformed/trailing/unknown fields | PASS |
| Explicit zero coordinates | Accepted | Register and assign at (0,0) | PASS |
| SQLite write-through | Active/busy then completed/available rows | SQL queries after creation and completion | PASS |
| Assignment GET | Matching assignment ID | HTTP test and compiled-server curl | PASS |
| Full compiled-server flow | Register two, assign, GET, complete, health | Actual TCP curl calls below | PASS: 200, 200, 201, 200, 200, 200 |

## Scope and limitations

Tests exercise Redis Lua concurrency using independent server instances sharing the real Redis connection pool, so the implementation's process-local mutation mutex cannot serialize the claim checks. They do not simulate separate OS processes for concurrency. The race detector covers Go memory accesses; Redis invariants are checked separately by response IDs and rider hashes.

SQLite write-through was verified for successful creation and completion. Redis/SQLite crash atomicity, outage recovery, and Redis rehydration were not exercised; those are outside this requested verification. No unverified result is claimed for those behaviors. No compile or vet warnings were observed.

## Follow-up check: Dockerfile build command and resulting binary (post-handoff)

Docker itself (the daemon) is still not available in this environment, so `docker build` / `docker compose up` were still not run directly. To close that gap as far as possible without Docker, the coordinator reproduced the Dockerfile's build stage by hand and ran the exact resulting binary:

```sh
cp go.mod go.sum *.go /tmp/docker-sim-build/
cd /tmp/docker-sim-build
go mod download
CGO_ENABLED=1 go build -trimpath -o out/rider-system .
```

This is the literal build command from the Dockerfile's build stage (`RUN CGO_ENABLED=1 go build -trimpath -o /out/rider-system .`); it exited 0 and produced a working binary.

That binary was then run directly (not in a container) against the real local Redis, with the same environment variables the compose file sets, and exercised through the full flow:

```sh
REDIS_ADDR=127.0.0.1:6379 HTTP_PORT=48123 SQLITE_PATH=/tmp/docker-sim-build/data/riders.db ./out/rider-system &
curl -X PUT  .../riders/docker-check-r1/location   -> HTTP 200
curl -X POST .../assignments                        -> HTTP 200 (201 on creation)
curl       .../assignments/<id>                     -> HTTP 200
curl -X POST .../assignments/<id>/complete           -> HTTP 200, status "completed"
curl       .../health                               -> HTTP 200, {"status":"ok"}
kill -TERM <pid>                                     -> exited 0
```

All five calls returned the expected status and body, and the SQLite file was created and written correctly at the configured path.

This confirms the build command, the env var wiring (`REDIS_ADDR`, `HTTP_PORT`, `SQLITE_PATH` match `config.go` exactly), and the compiled binary's runtime behavior are all correct — the same logic the Docker image would run. What remains genuinely unverified is Docker/Compose-specific: inter-container DNS (the `redis` service hostname), the healthcheck gating `depends_on`, and the non-root user's write permission on the named volume. Those are standard, low-risk patterns, but only an actual `docker compose up --build` run confirms them — still recommended before relying on the container path.

## Exact smoke transcript

```text
REDIS_ADDR=127.0.0.1:6379 HTTP_PORT=48817 SQLITE_PATH=/home/agent/rider-system/.verification-smoke/smoke.db ./rider-system &

curl -sS -w '
HTTP %{http_code}
' -X PUT http://127.0.0.1:48817/riders/verification-smoke-r1/location -H 'Content-Type: application/json' -d '{"latitude":12.345,"longitude":45.678}'

{"id":"verification-smoke-r1","latitude":12.345,"longitude":45.678,"h3_cell":"8752c01a6ffffff","available":true,"current_assignment_id":null,"updated_at":"2026-11-04T16:44:37.420020649Z"}

HTTP 200

curl -sS -w '
HTTP %{http_code}
' -X PUT http://127.0.0.1:48817/riders/verification-smoke-r2/location -H 'Content-Type: application/json' -d '{"latitude":12.346,"longitude":45.678}'

{"id":"verification-smoke-r2","latitude":12.346,"longitude":45.678,"h3_cell":"8752c01a6ffffff","available":true,"current_assignment_id":null,"updated_at":"2026-11-04T16:44:37.447481655Z"}

HTTP 200

curl -sS -w '
HTTP %{http_code}
' -X POST http://127.0.0.1:48817/assignments -H 'Content-Type: application/json' -d '{"order_id":"verification-smoke-order","pickup":{"latitude":12.345,"longitude":45.678},"drop":{"latitude":12.35,"longitude":45.68}}'

{"id":"9bd5ed24e35ed1b647bdbd8703ef9537","order_id":"verification-smoke-order","rider_id":"verification-smoke-r1","pickup":{"latitude":12.345,"longitude":45.678},"drop":{"latitude":12.35,"longitude":45.68},"pickup_h3_cell":"8752c01a6ffffff","drop_h3_cell":"8752c01a6ffffff","status":"active","created_at":"2026-11-04T16:44:37.470866339Z","completed_at":null}

HTTP 201

curl -sS -w '
HTTP %{http_code}
' http://127.0.0.1:48817/assignments/9bd5ed24e35ed1b647bdbd8703ef9537

{"id":"9bd5ed24e35ed1b647bdbd8703ef9537","order_id":"verification-smoke-order","rider_id":"verification-smoke-r1","pickup":{"latitude":12.345,"longitude":45.678},"drop":{"latitude":12.35,"longitude":45.68},"pickup_h3_cell":"8752c01a6ffffff","drop_h3_cell":"8752c01a6ffffff","status":"active","created_at":"2026-11-04T16:44:37.470866339Z","completed_at":null}

HTTP 200

curl -sS -w '
HTTP %{http_code}
' -X POST http://127.0.0.1:48817/assignments/9bd5ed24e35ed1b647bdbd8703ef9537/complete

{"id":"9bd5ed24e35ed1b647bdbd8703ef9537","order_id":"verification-smoke-order","rider_id":"verification-smoke-r1","pickup":{"latitude":12.345,"longitude":45.678},"drop":{"latitude":12.35,"longitude":45.68},"pickup_h3_cell":"8752c01a6ffffff","drop_h3_cell":"8752c01a6ffffff","status":"completed","created_at":"2026-11-04T16:44:37.470866339Z","completed_at":"2026-11-04T16:44:37.516297468Z"}

HTTP 200

curl -sS -w '
HTTP %{http_code}
' http://127.0.0.1:48817/health

{"status":"ok"}

HTTP 200

kill -TERM 2100393
# Server exited with status 0
```

## Final checks and cleanup

`go build ./...` and `go vet ./...` both exited 0 with no output, including the final rerun. The first race run passed (`ok rider-system 1.388s`). After adjusting the test helper to place temporary SQLite databases inside the workspace, the final command was:

```sh
set -o pipefail
REDIS_ADDR=127.0.0.1:6379 go test ./... -v -race | tee .verification-smoke/test-output.txt
```

It exited 0 with no race report:

```text
PASS
ok  	rider-system	1.421s
```

The full final test output is retained in `.verification-smoke/test-output.txt`; the smoke runner, transcript, and server log are also retained there. Smoke Redis records were removed with these exact commands (results 2 and 4 respectively):

```sh
redis-cli -h 127.0.0.1 -p 6379 SREM dispatch:cell:8752c01a6ffffff verification-smoke-r1 verification-smoke-r2
redis-cli -h 127.0.0.1 -p 6379 DEL dispatch:rider:verification-smoke-r1 dispatch:rider:verification-smoke-r2 dispatch:order:verification-smoke-order dispatch:assignment:9bd5ed24e35ed1b647bdbd8703ef9537
```

The smoke SQLite database and any WAL/SHM sidecars were removed after the server exited. No commits were made.
