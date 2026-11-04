# Prompt history

This file records the original take-home prompt given to Claude, and the
substantive prompts Claude then gave to Codex agents to implement, verify,
and document this project. Nothing here has been edited for content beyond
light formatting; these are the actual prompts used in this build.

## 1. Original prompt (given to Claude)

<details>
<summary>Full original take-home prompt</summary>

```text
Build a small Go API project for a rider management interview take-home. The interviewer explicitly requested an AI-assisted implementation requiring roughly 30 minutes of my involvement.

Keep it simple, working, independently implemented, well documented, and easy to explain. Build the project; do not stop at a plan.

## 1. Your role and use of Codex

You are the project coordinator. Use Codex agents for implementation and testing.

You own:
- Scope and design decisions.
- Clear task instructions for Codex.
- Coordination and final code review.
- Documentation accuracy.
- Final verification and handoff.

Give implementation and verification distinct responsibilities. Keep agent coordination proportional to this small project and avoid having multiple agents edit the same files simultaneously.

If Codex is unavailable, state that clearly and proceed with the tools available. Never claim to have used an agent or executed a check that you did not use or run.

Start with a short plan and proceed immediately. Resolve routine choices yourself. Ask only questions that genuinely block implementation.

## 2. Background and project origin

Use this account as the factual starting point for the documentation:

"In a system design interview, I was asked to design a rider management system. Drawing on previous experience, I proposed Redis mappings from H3 cells to rider IDs and from each rider to their current H3 cell.

The interviewer asked why I would not use a database. I explained that H3 allows us to calculate neighboring cells and use direct Redis lookups to retrieve candidate riders. The interviewer accepted the direction and requested a small AI-assisted implementation, with roughly half an hour of my involvement, to be shared through GitHub.

I then discussed the take-home with ChatGPT and requested a prompt for Claude to coordinate Codex agents for implementation and testing. I specified Go, simple APIs, pickup and drop coordinates resolved to H3, and a narrow rider-assignment scope.

I also required an independent implementation. An existing local system could provide broad conceptual inspiration only, with no copying. I subsequently requested documentation covering the project, the development process, the prompts, and the reasoning behind the design."

Do not invent interviewer quotations, additional interview events, or conversations.

Clarify in the technical explanation that:
- A database could also support H3-based lookups.
- Redis is a reasonable choice for this demo's direct access patterns and atomic state transitions.
- H3 groups spatial candidates; it does not establish exact geographic distance.
- This design does not prove that Redis is universally better than a database.

## 3. Originality and existing local code

There may be an existing rider system on this laptop. Inspection is optional and must not block this project.

If I explicitly provide its location:
- Treat it as read-only.
- Inspect only what is needed for broad conceptual understanding.
- Do not copy, translate, adapt, or reproduce its code, schemas, scripts, naming conventions, configuration, or architecture.
- Do not expose its contents through agents, logs, prompts, documentation, commits, or the new repository.
- Give Codex agents only this specification and the newly created project.

Build an independent implementation from this specification and public dependencies. Do not imply that you inspected the existing system unless you actually did.

## 4. Functional scope

Implement only this flow:

1. Register a rider implicitly through a location update.
2. Convert the rider's coordinates to an H3 cell.
3. Maintain the rider's current location and H3 membership in Redis.
4. Accept an assignment request with an order ID, pickup coordinates, and drop coordinates.
5. Resolve pickup and drop to H3 cells.
6. Find an available rider near pickup.
7. Assign that rider atomically.
8. Complete the assignment and make the rider available again.

This is an API-only project. No frontend application.

The HTML documentation described later is a static project walkthrough, not a product UI.

## 5. Technology and structure

Use:
- Go.
- Redis as the only runtime datastore.
- A maintained H3 library compatible with Go.
- The standard HTTP library unless a small dependency materially simplifies the implementation.
- Docker Compose for straightforward local execution.

Use a fixed H3 resolution, defaulting to 7, with the value defined centrally.

Prefer a small, readable project structure. Avoid elaborate architectural layers, generic repository frameworks, or unnecessary abstractions.

Choose compatible dependencies and ensure the Docker build includes any native requirements of the chosen H3 library.

## 6. Redis location model

Maintain:
- H3 cell → set of rider IDs.
- Rider ID → current H3 cell, actual coordinates, availability, and current assignment reference when applicable.
- Minimal assignment records and an order-to-assignment mapping for idempotency.

Keep all riders in their current cell sets and filter by availability during assignment.

Store actual coordinates alongside H3 cells. Use H3 for candidate discovery and actual coordinates for distance ranking.

When a rider moves:
- Remove their membership from the previous cell.
- Add membership to the new cell.
- Update their stored cell and coordinates consistently.
- Preserve their assignment and availability state.

Use a consistent Redis key prefix and document the exact final key model.

For this demo, riders do not expire automatically. Document the consequences of stale locations and explain that Redis durability depends on its configuration.

## 7. Minimal API

Use these endpoints or equally simple equivalents.

### PUT /riders/{rider_id}/location

Example request:

{
  "latitude": 28.4595,
  "longitude": 77.0266
}

Behavior:
- Creates an available rider if the rider does not exist.
- Updates coordinates and H3 membership for an existing rider.
- Does not make an assigned rider available.
- Returns the rider's current stored state.

### POST /assignments

Example request:

{
  "order_id": "order-123",
  "pickup": {
    "latitude": 28.4595,
    "longitude": 77.0266
  },
  "drop": {
    "latitude": 28.4720,
    "longitude": 77.0500
  }
}

Return:
- Assignment ID.
- Order ID.
- Rider ID.
- Pickup and drop coordinates.
- Pickup and drop H3 cells.
- Assignment status.

Repeated requests for the same order and same payload must return the existing assignment, including under concurrency.

If the same order ID is reused with conflicting pickup or drop data, return a conflict.

### GET /assignments/{assignment_id}

Return the stored assignment or an appropriate not-found response.

### POST /assignments/{assignment_id}/complete

Behavior:
- Marks the assignment completed.
- Releases its associated rider.
- Repeated completion is harmless.
- Does not release a rider who is associated with a different assignment.

### GET /health

Provide a basic health check that verifies Redis connectivity.

## 8. Candidate lookup and assignment

Use a small bounded H3 neighborhood around pickup. Make the maximum grid distance configurable, with a small default such as 2.

For simplicity:
1. Calculate the pickup cell and cells within the configured grid distance.
2. Collect rider IDs from those cells.
3. Read candidate rider state.
4. Filter for available riders.
5. Rank candidates by straight-line distance from their actual coordinates to pickup.
6. Attempt to claim candidates in that order.
7. If a candidate was claimed concurrently, try the next candidate.
8. Return a clear no-rider-available response when none can be claimed.

Use a deterministic tie-breaker such as rider ID.

Keep the search bounded. Do not expand indefinitely or claim that this finds the globally nearest rider.

Candidate discovery and ranking may use a snapshot of moving riders. Explain this limitation without adding a complex synchronization system.

The drop location is stored and converted to H3. Do not implement routing, travel-time estimation, or route optimization.

## 9. Atomicity and idempotency

Use small Redis Lua scripts or an equally clear Redis atomic mechanism for:
- Updating rider location and cell membership consistently.
- Claiming an available rider, recording the assignment, and registering the order-to-assignment mapping.
- Completing an assignment and safely releasing its rider.

Required invariants:
- One rider cannot hold two active assignments.
- One order cannot produce multiple assignments.
- Retrying a successful assignment request returns the existing assignment even if no riders are currently available.
- Conflicting payloads for an existing order return a conflict.
- Completing an old assignment cannot release a rider from a newer assignment.
- Location updates cannot overwrite assignment state.

Check order idempotency before candidate lookup and again during the atomic claim to handle concurrent requests.

Keep this scoped to one Redis instance. Redis Cluster compatibility is out of scope.

## 10. Validation and errors

Use clear JSON responses and appropriate HTTP status codes.

Validate:
- Required identifiers.
- Required coordinate fields, including distinguishing a missing field from valid zero coordinates.
- Latitude and longitude ranges.
- Malformed JSON.

Handle Redis failures without returning false success. Configure reasonable server and Redis operation timeouts.

Keep credentials out of the repository.

## 11. Explicit exclusions

Do not add:
- SQL or another database.
- Kafka, queues, or background workers.
- Microservices.
- Authentication or authorization.
- Payments or notifications.
- Live tracking streams.
- A frontend or dashboard.
- Kubernetes or Terraform.
- CI pipelines or observability infrastructure.
- Automatic reassignment or cancellation.
- A full order lifecycle.
- Routing or ETA calculation.
- Production-grade distributed infrastructure.

Keep the implementation small enough that I can read its core code and explain the system in five minutes.

## 12. Testing and verification

Use Codex for implementation and independent verification when available.

Write a few meaningful tests covering:
- Rider creation.
- Location updates within a cell and across cell boundaries.
- Removal from the previous cell.
- Preservation of assigned status during location updates.
- Correct pickup and drop H3 conversion.
- Assignment from a neighboring cell.
- No available rider.
- Concurrent requests cannot double-assign one rider.
- Concurrent retries for one order return one assignment.
- Conflicting payloads for one order return a conflict.
- Completion releases the rider.
- Repeated completion is harmless.
- Invalid input produces useful errors.

Prefer integration tests against real Redis for Lua scripts and concurrency behavior. Do not rely entirely on mocks.

Run the project and relevant tests if the environment permits. Check the documented curl flow against the running API.

Record commands actually executed and report any checks that could not run. Do not invent test output, coverage, timings, or performance figures.

## 13. Documentation deliverables

Documentation is a core deliverable.

Create:
- `README.md`: concise setup, usage, and limitations.
- `docs/index.html`: a polished, self-contained project walkthrough.
- `docs/prompts.md`: this complete initial prompt and the substantive prompts subsequently given to Codex agents.

Document every endpoint, Redis key, configuration option, important state transition, design decision, and verification result. Keep explanations accurate and avoid repetitive filler.

### HTML presentation requirements

Create one responsive HTML file with:
- Embedded CSS.
- Clear navigation or a sidebar.
- Readable typography.
- A restrained professional visual style.
- Useful diagrams.
- Tables for structured comparisons and test results.
- Readable code blocks.
- Collapsible sections for long prompts and detailed logs.

It must open directly in a browser without a build step or external services. Use inline SVG or HTML/CSS for diagrams so it works offline.

Include these sections:

### A. Project overview

Explain:
- The assignment.
- The implemented scope.
- The end-to-end rider and assignment flow.
- The main components and their responsibilities.
- Explicit limitations.

Include an architecture diagram and a rider-state diagram matching the final implementation.

### B. How the project started

Use the factual account supplied in section 2.

Explain how the interview discussion became a small Go and Redis implementation, then an AI-assisted build coordinated by Claude using Codex where available.

Do not invent a timeline or claim the work took 30 minutes unless that was measured. The 30-minute figure describes the intended level of my involvement.

### C. Requirements and design discussion

Present thoughtful design questions and concise answers, including:
- Why Redis for this demo?
- Could a database support the same H3 approach?
- Why keep both mapping directions?
- Why retain coordinates alongside H3 cells?
- How far does candidate search extend?
- How is the nearest candidate selected?
- What happens when two requests choose the same rider?
- What happens when an order request is retried?
- How does rider movement update cell membership?
- How does completion release a rider safely?
- Why store drop location without route optimization?
- What was excluded and why?

Label the source of each decision:
- User requirement.
- Proposed design from the ChatGPT-generated specification.
- Implementation decision made during the build.
- Confirmed clarification from an actual question I answered.

You may create polished retrospective questions and answers, but place them under "Reconstructed design rationale" and explicitly identify them as explanatory summaries rather than a verbatim conversation.

Never invent my answers or portray agent-selected choices as decisions I explicitly made.

If genuine clarifications occur, record the actual question, my answer, and its effect on the implementation. Do not ask unnecessary questions just to populate the documentation.

### D. Prompting and agent workflow

Include:
- This full initial prompt in a collapsible section.
- Substantive prompts sent to Codex agents.
- Claude's responsibilities.
- Each agent's actual responsibilities.
- Meaningful findings, revisions, and resolutions.

Describe the workflow that actually happened. If Codex or agent delegation was unavailable, say so.

Do not include secrets, proprietary code, sensitive local paths, or unrelated conversation content.

### E. Implementation walkthrough

Explain the final implementation:
- Project structure and relevant file paths.
- Endpoint contracts and examples.
- Redis key names, types, and purposes.
- H3 resolution and neighborhood lookup.
- Distance ranking.
- Atomic movement, assignment, and completion.
- Order idempotency.
- Configuration.
- Error handling.

Use concise code excerpts only where they help. Ensure examples and diagrams match the final code.

### F. Verification and results

Include a table with:
- Scenario.
- Expected behavior.
- Verification method.
- Actual result.

Separate:
- Checks that passed.
- Failures discovered and fixed.
- Checks that could not run.

Include exact commands actually executed and concise relevant output.

### G. Run and review

Include:
- Prerequisites.
- One straightforward startup command.
- Copy-paste curl examples covering the entire flow.
- Test commands.
- A five-minute code-reading path.
- Known limitations.
- Brief future considerations, including durable database storage and stale-location handling.

Do not implement those future considerations.

### README requirements

Keep the README concise and include:
- What the project does.
- Startup command.
- Complete curl walkthrough.
- Test commands.
- Configuration options.
- Brief Redis and H3 explanation.
- Important limitations.
- Links to the HTML walkthrough and prompt history.
- An honest note about AI assistance.

Documentation must describe the final implementation. Record material deviations from this specification and their reasons.

Open and visually inspect the HTML if browser tooling is available. Check navigation, readability, diagrams, code-block overflow, and narrow-screen layout. If visual inspection is unavailable, report that limitation.

## 14. Repository and final handoff

Deliver:
- Working Go source.
- Dependency files.
- Dockerfile and Compose configuration.
- Focused tests.
- README.
- Self-contained HTML walkthrough.
- Prompt history.
- Appropriate `.gitignore`.

Initialize Git locally if needed. Do not include unrelated files, secrets, or existing system code.

If GitHub access is already available and the destination is unambiguous, create a private repository and push it. Otherwise leave the project ready to push and identify exactly what is missing. Do not let GitHub setup block implementation or verification.

Finish with a concise handoff covering:
- What works.
- How to run it.
- Tests actually executed and their results.
- Documentation paths.
- Known limitations.
- Repository path or URL.

Prioritize a coherent, working, explainable submission over additional features.
```

</details>

## 2. Prompt to Codex — implementation pass

Goal: implement the Go/Redis/H3/SQLite service. This agent did not write
tests or documentation; a separate agent verified the work.

<details>
<summary>Implementation prompt</summary>

```text
You are the IMPLEMENTATION agent for a small Go API project. Work only inside
this directory: /home/agent/rider-system (already `git init`'d, remote
`origin` set to a private GitHub repo). Do not touch any other directory on
this machine, and do not look for or reference any other project on this
machine — build this independently from the spec below and public Go
packages only.

Toolchain already installed and on PATH: `go` (1.23.4, linux/arm64, at
~/.local/bin/go), `gcc`/`make` (for CGO), `redis-server`/`redis-cli` (built
from source, at ~/.local/bin). No Docker is installed on this machine — you
cannot run `docker build`/`docker compose up` here, just write correct
Dockerfile/compose files; a human will build those elsewhere.

## Task

Implement the Go service described below. This is PASS 1 of 2: you own
implementation only. A separate verification agent will write and run the
integration tests afterward — do not write tests yourself, but DO write a
short internal file `NOTES-impl.md` (not a deliverable, just for the next
agent and the coordinator) listing: the exact Redis key model you ended up
with, the H3 library and version you chose and why, every implementation
decision you made that the spec left open, and any deviation from the spec
with its reason.

### Functional scope

1. Register a rider implicitly through a location update.
2. Convert the rider's coordinates to an H3 cell.
3. Maintain the rider's current location and H3 membership in Redis.
4. Accept an assignment request with an order ID, pickup coordinates, and
   drop coordinates.
5. Resolve pickup and drop to H3 cells.
6. Find an available rider near pickup.
7. Assign that rider atomically.
8. Complete the assignment and make the rider available again.

API-only. No frontend.

### Technology

- Go, standard `net/http` (no web framework needed).
- Redis for the hot path: cell membership, candidate lookup, availability,
  and ALL atomicity (claim/complete/location-move Lua scripts). This is
  non-negotiable — Redis stays the live operational engine and the only
  place atomic claims happen; do not move candidate lookup or the atomic
  claim to SQLite.
- SQLite as a durable system-of-record, added on top of Redis for
  durability (Redis alone only persists if configured to, which the spec
  originally flagged as a limitation). Use `modernc.org/sqlite` (pure Go
  driver, no extra CGO beyond what H3 already needs — keeps the Docker
  build simple). Schema: a `riders` table (id, lat, lon, h3_cell,
  available, current_assignment_id, updated_at) and an `assignments` table
  (id, order_id unique, rider_id, pickup/drop lat+lon+cell, status,
  created_at, completed_at). After each successful Redis atomic operation
  (location update, claim, complete) synchronously write-through the
  resulting state to SQLite in the same request — no background worker,
  no queue. SQLite is written-to but NOT read from by the live API in this
  scope: Redis remains authoritative for all reads/lookups. Document
  explicitly that rehydrating Redis from SQLite on restart is NOT
  implemented (list it as a future consideration) — only forward durability
  of writes is in scope. Use `github.com/redis/go-redis/v9`.
- A maintained H3 library for Go — use `github.com/uber/h3-go/v4` (CGO,
  vendors the H3 C source, so it builds with just a C compiler, no system
  libh3 package needed). Confirm `go build ./...` actually works with it
  before moving on; if it genuinely cannot build in this environment, pick
  the next best maintained alternative and record why in NOTES-impl.md.
- Fixed H3 resolution, default 7, defined as a single named constant/config
  value, not scattered magic numbers.
- Docker Compose file for local `docker compose up` (service + redis). You
  cannot run this here, just make it correct and consistent with the actual
  Dockerfile and env vars you use.

### Redis data model (design this precisely; document the final version)

- `H3 cell -> set of rider IDs` currently in that cell.
- `rider ID -> hash`: current H3 cell, latitude, longitude, availability,
  current assignment ID (if any).
- An assignment record per assignment ID (order ID, rider ID, pickup/drop
  coords + H3 cells, status, timestamps).
- `order ID -> assignment ID` mapping for idempotency.
- Use one consistent key prefix, e.g. `rider:`, `cell:`, `assignment:`,
  `order:`. Document the exact final keys, their Redis type, and TTL (none
  needed for this demo — riders do not expire automatically; note the
  consequence of stale locations in NOTES-impl.md).

Rider movement must, atomically:
- remove membership from the previous cell set,
- add membership to the new cell set,
- update stored cell + coordinates,
- preserve availability and current assignment reference (a location update
  must never silently clear or overwrite assignment state).

### API

`PUT /riders/{rider_id}/location`
Request: `{"latitude": float, "longitude": float}`.
- Creates an available rider if the rider does not exist.
- Updates coordinates + H3 membership for an existing rider.
- Does NOT make an already-assigned rider available again.
- Returns the rider's current stored state (id, lat, lon, h3 cell,
  available, current assignment id or null).

`POST /assignments`
Request: `{"order_id": string, "pickup": {"latitude":..,"longitude":..},
"drop": {"latitude":..,"longitude":..}}`.
Response: assignment id, order id, rider id, pickup/drop coords, pickup/drop
H3 cells, status.
- Idempotent on `order_id`: a retried request with the SAME payload for an
  order that already has an assignment returns that SAME assignment
  (same assignment id), even if no rider is currently available (i.e. check
  the order->assignment mapping BEFORE doing candidate lookup).
- A retried request for an existing `order_id` with a DIFFERENT pickup or
  drop returns a 409 conflict, not a new assignment.
- Must hold correctly under concurrent duplicate requests for the same
  order — check idempotency again inside whatever atomic claim mechanism you
  use, not just once up front.

`GET /assignments/{assignment_id}` — the stored assignment, or 404.

`POST /assignments/{assignment_id}/complete`
- Marks the assignment completed and releases its rider (available again).
- Idempotent: completing an already-completed assignment is harmless (not
  an error, no double side effects).
- Must NOT release a rider if that rider has since moved on to a newer,
  different assignment — only release if the rider's current assignment
  reference still points at THIS assignment id.

`GET /health` — checks Redis connectivity (e.g. PING), returns ok/degraded.

### Candidate lookup and assignment

1. Compute the pickup H3 cell, plus all cells within a configurable grid
   distance (default 2; use the H3 "grid disk" function). Make this a config
   value, not hardcoded.
2. Union the rider-ID sets of those cells.
3. Fetch each candidate rider's state; keep only ones marked available.
4. Rank by straight-line (haversine or simple euclidean on lat/lon — pick
   one, document it) distance from the rider's actual stored coordinates to
   the pickup point. Tie-break deterministically by rider ID (e.g. lexical
   order) so results are reproducible.
5. Try to atomically claim candidates in that ranked order: first one that
   successfully claims wins; if a candidate was already claimed by a
   concurrent request, move to the next.
6. If no candidate can be claimed, return a clear, distinct "no rider
   available" response (e.g. 409 or 404 with a specific error code/body —
   your choice, just document it) rather than a generic error.

This is a bounded, snapshot-based search — explicitly NOT a claim to find the
globally nearest rider, and a rider's position may be in flux between the
snapshot read and the claim attempt. That's an accepted limitation, not a bug
to fix with extra synchronization.

Drop coordinates are stored and converted to an H3 cell purely for the
record — no routing, ETA, or distance-to-drop logic at all.

### Atomicity

Use Redis Lua scripts (`EVAL`/`EVALSHA` via go-redis) for:
- the location-update (cell membership move + coordinate update, preserving
  assignment/availability state) as one atomic operation,
- the claim-a-rider-and-create-an-assignment operation (must check
  order-id idempotency again inside the script, check the candidate is
  still available, and atomically mark them unavailable + write the
  assignment + write the order->assignment mapping),
- the complete-assignment operation (idempotent complete; only clears the
  rider's assignment ref if it still matches this assignment id).

Required invariants (make these true, not just documented):
- A rider can never hold two simultaneous active assignments.
- An order can never produce more than one assignment.
- Conflicting payloads on a repeated order id -> 409, never a new
  assignment.
- Completing an old/stale assignment can never release a rider who has
  since been reassigned.
- Location updates never clobber assignment/availability state.

Single Redis instance only; no cluster-mode concerns.

### Validation and errors

- JSON error responses with clear codes/messages and sensible HTTP status
  codes (400 bad input, 404 not found, 409 conflict, 503 for Redis being
  down, etc).
- Distinguish a MISSING coordinate field from a valid `0,0` coordinate
  (don't just check for Go zero-value — use pointers or a presence check).
- Validate latitude in [-90,90], longitude in [-180,180].
- Malformed JSON -> 400 with a useful message.
- Redis errors must surface as real failures (503-ish), never a false
  "success".
- Add reasonable timeouts on the HTTP server and on Redis operations (context
  with timeout on every Redis call).

### Explicitly out of scope — do not add any of this

Queues/background workers, microservices, auth, payments,
notifications, live tracking streams, a frontend/dashboard, Kubernetes/
Terraform, CI pipelines, observability infra, automatic reassignment/
cancellation, a full order lifecycle, routing/ETA, Redis Cluster support.

### Deliverables for this pass

- `go.mod`/`go.sum`, a small flat package layout (don't over-architect; a
  `main.go` plus a handful of focused files is enough — no repository-
  pattern/DI-framework ceremony).
- Config via environment variables with sane defaults (Redis addr, HTTP
  port, H3 resolution, grid distance, SQLite file path), defined centrally.
  Run SQLite migrations/`CREATE TABLE IF NOT EXISTS` automatically on
  startup — no separate migration tool.
- `Dockerfile` (multi-stage, must actually support building the CGO-based H3
  dependency — document the base image choice and why).
- `docker-compose.yml` (service + redis, correct env wiring).
- `.gitignore` (Go build artifacts, etc — no secrets to ignore here since
  there are none).
- `NOTES-impl.md` as described above.

Run `go build ./...` and `go vet ./...` yourself and fix everything until
both are clean. Do not write `README.md` or `docs/` — later passes handle
those. Do not `git commit` — the coordinator will review and commit.

When done, print a short summary of exactly what you built, the final file
tree, and confirmation that `go build`/`go vet` passed.
```

</details>

**Result:** `go build ./...` and `go vet ./...` passed. Produced `main.go`,
`models.go`, `scripts.go`, `config.go`, `storage.go`, `Dockerfile`,
`docker-compose.yml`, `.gitignore`, `.dockerignore`, and `NOTES-impl.md`
(internal handoff notes). No tests, docs, or commits, per instructions.

## 3. Prompt to Codex — verification pass

Goal: write and run integration tests against a real Redis instance,
fix any genuine bugs found (none were), and record exact results.

<details>
<summary>Verification prompt</summary>

```text
You are the VERIFICATION agent for a small Go API project. A separate
implementation agent already built it in /home/agent/rider-system (that
agent's notes, if any, are in NOTES-impl.md). You did not write the
implementation — review it fresh and verify it against the spec below. Fix
only genuine bugs you find against this spec; do not redesign working code
or add scope.

Work only inside /home/agent/rider-system.

Toolchain on PATH: `go` 1.23.4 linux/arm64, `gcc`/`make`, `redis-cli`.
A real Redis server is already running and reachable at 127.0.0.1:6379
(started by the coordinator before this task). Use that real instance for
integration tests — do not mock Redis for the Lua-script/concurrency tests.
If the implementation's Redis client config doesn't default to
127.0.0.1:6379, set REDIS_ADDR=127.0.0.1:6379 (or whatever env var the code
uses) when running tests.

## Task

1. Read the existing code and NOTES-impl.md first to understand what was
   actually built (Redis key model, H3 library used, the SQLite durability
   layer, Lua scripts, API shapes).
2. Write Go tests (table-driven where sensible) covering, as integration
   tests against the real Redis instance above:
   - Rider creation via location update.
   - Location update within the same H3 cell and across a cell boundary
     (verify old-cell membership is actually removed).
   - A location update preserves an existing assignment/unavailable state
     (does not silently free an assigned rider).
   - Correct H3 conversion for pickup and drop.
   - Successful assignment to a rider in a neighboring (not just the exact
     pickup) cell.
   - No-rider-available response when nothing is in range or available.
   - Concurrent assignment requests for riders near the same pickup cannot
     double-assign the same rider (run several goroutines hitting the
     claim path concurrently, assert only one wins per rider).
   - Concurrent retries of the identical order_id+payload return exactly
     one assignment (same assignment id) across all callers.
   - A retried order_id with a DIFFERENT pickup or drop returns a conflict,
     not a new assignment.
   - Completion releases the rider (becomes available again).
   - Repeated completion of the same assignment is harmless (no error, no
     double side effects).
   - Completing an old assignment does not release a rider who has since
     been claimed by a newer assignment for that same rider.
   - Invalid input (missing fields, out-of-range lat/lon, malformed JSON,
     missing vs. zero coordinates) produces clear 4xx errors, not panics or
     500s.
   - If the SQLite durability layer exists: after an assignment is created
     and completed, the corresponding SQLite rows reflect that (write-
     through actually happened).
3. Run `go build ./...`, `go vet ./...`, and `go test ./... -v -race` and
   fix any real bug you find (not a test-authoring mistake). If a test
   reveals the implementation violates one of the required invariants
   (double-assignment, double-release, lost idempotency, etc.), fix the
   implementation, not the test.
4. Also do a quick end-to-end smoke check by actually running the compiled
   server against the live Redis instance (pick a free port, start it in
   the background, curl through the full flow: register two riders via
   location updates, create an assignment, GET it, complete it, confirm
   GET /health works), then stop the server. Capture the exact curl
   commands and responses you used.
5. Write `VERIFICATION.md` (a working file for the coordinator, not
   necessarily a final deliverable — the coordinator will fold it into
   docs) containing:
   - Exact commands you ran (go build/vet/test, the curl smoke flow).
   - A table: scenario | expected | method | actual result.
   - Anything that passed cleanly.
   - Any real bug found and the fix you made (describe the bug and the fix
     briefly, file/line).
   - Anything you could NOT verify in this environment, and why (be
     honest — do not claim a check passed if you didn't actually run it).
6. Fix any actual compile warnings / vet issues you introduced or found.

Do not touch anything outside /home/agent/rider-system. Do not invent test
output — only report what you actually ran and saw. Do not `git commit` —
the coordinator reviews and commits. When done, print a summary of what you
tested, what (if anything) you fixed in the implementation, and the final
`go test` result.
```

</details>

**Result:** Added `verification_test.go`. All tests passed, including
concurrent-claim and concurrent-retry races under `-race`, idempotency and
conflict handling, movement/cell-membership correctness, completion safety,
input validation, and SQLite write-through. No implementation bug was
found; no production code was changed. A compiled-server curl smoke flow
was also run end to end. Full results: `VERIFICATION.md`.

## 4. Prompt to Codex — documentation pass

Goal: draft `README.md` and the self-contained `docs/index.html` walkthrough
from the actual implementation and verification evidence, labeling design
decisions by their real source (user requirement, proposed spec, build-time
implementation decision, or confirmed clarification).

<details>
<summary>Documentation prompt</summary>

```text
You are the DOCUMENTATION agent for a small Go API project at
/home/agent/rider-system. The implementation and verification passes are
done — read NOTES-impl.md and VERIFICATION.md (both already in the repo)
before writing anything, plus the actual source files, so every claim you
write matches what was really built and really tested. Do not invent test
results, timings, or coverage numbers that aren't in VERIFICATION.md. Do not
claim Docker was actually run — NOTES-impl.md says Docker could not be
executed in this environment.

Work only inside /home/agent/rider-system.

## Factual project-origin account (use verbatim, do not embellish or add
invented interviewer quotes/events)

"In a system design interview, I was asked to design a rider management
system. Drawing on previous experience, I proposed Redis mappings from H3
cells to rider IDs and from each rider to their current H3 cell. The
interviewer asked why I would not use a database. I explained that H3
allows us to calculate neighboring cells and use direct Redis lookups to
retrieve candidate riders. The interviewer accepted the direction and
requested a small AI-assisted implementation, with roughly half an hour of
my involvement, to be shared through GitHub. I then discussed the take-home
with ChatGPT and requested a prompt for Claude to coordinate Codex agents
for implementation and testing. I specified Go, simple APIs, pickup and
drop coordinates resolved to H3, and a narrow rider-assignment scope. I
also required an independent implementation. An existing local system could
provide broad conceptual inspiration only, with no copying. I subsequently
requested documentation covering the project, the development process, the
prompts, and the reasoning behind the design."

One real deviation from the original spec happened mid-build: the person
explicitly said, mid-conversation, "use sqlite as the db... only redis is
not the correct way," which conflicted with the spec's original "Redis
only, no SQL" rule. When Claude (the coordinator) flagged that conflict
against the interview backstory, the person chose, from three options
Claude offered: keep Redis as the live/candidate-lookup/atomicity engine
exactly as designed, and add SQLite purely as a durable write-through
system-of-record for riders/assignments (not a replacement, not used for
candidate lookup or atomic claims). This is a CONFIRMED CLARIFICATION — label
it as such in the design-discussion doc, with the real question/answer, not
as an invented retrospective rationale.

A separate real environment fact, not an interview event: Codex's sandboxed
execution (bubblewrap) does not work on the build machine (Ubuntu 24.04
restricts unprivileged user namespaces by default), so Codex had to be run
unsandboxed, explicitly scoped to this project directory only, after the
person confirmed that was fine. Mention this plainly in the agent-workflow
section as an environment/tooling fact, not an interview event.

## Deliverables

### 1. `README.md` (repo root)

Concise. Include: what the project does; startup command (note Docker
Compose was written but not executed in the build environment — the human
should verify `docker compose up` themselves); a complete curl walkthrough
covering the full flow (register two riders via location updates, create
an assignment, GET it, complete it, GET /health); test commands actually
used (`go test ./... -v -race` against a real Redis instance; note how to
start one, e.g. `redis-server`); configuration options table (env vars +
defaults, pulled from config.go); a short Redis+H3 explanation; important
limitations (bounded/snapshot candidate search, no Redis->SQLite
rehydration on restart, single Redis instance only, no auth); links to
`docs/index.html` and `docs/prompts.md`; an honest one-paragraph note that
this was built with AI assistance (Claude coordinating Codex agents) with
roughly 30 minutes of direct human involvement by design, and a link to
the prompt history for anyone who wants the detail.

### 2. `docs/prompts.md`

Will be assembled by the coordinator (not you) from the actual prompt texts
used. Skip this file — do not create it.

### 3. `docs/index.html`

A single self-contained, responsive HTML file. Embedded CSS only (no
external fonts/CDN/scripts — must open offline in a browser with no build
step). Inline SVG for diagrams (no canvas/JS chart libs needed). Sidebar or
clear top navigation. Collapsible `<details>` sections for long prompt text
and detailed logs. Tables for the config/key-model/verification-results
comparisons. Readable `<pre><code>` blocks with horizontal scroll handling
on narrow screens (don't let code overflow break mobile layout). A single
dark, deliberate color theme is fine — do not attempt a generic
light-corporate template; keep it restrained and professional, good
contrast, readable serif or system-ui headings, system-ui/monospace body
and code. `<meta name="viewport" content="width=device-width,initial-scale=1">`.
A `<link rel="icon">` emoji favicon (pick one that fits a rider/dispatch
system, e.g. 🛵 or 🧭).

Sections, in this order, matching section headers A–G from the original
spec (paraphrase the headers, don't need the letters):

**A. Project overview** — the assignment, implemented scope, end-to-end
flow, main components (HTTP handlers, Redis Lua scripts, H3 lookup, SQLite
write-through) and their responsibilities, explicit limitations. Include:
 - An inline-SVG architecture diagram: client -> Go HTTP server -> {Redis
   (cell sets, rider hashes, assignment hashes, order map, Lua scripts),
   SQLite (durable riders/assignments)}.
 - An inline-SVG rider-state diagram: states are roughly
   unregistered -> available -> assigned -> available again (completion),
   with the location-update edge that can hit "available" or "assigned"
   without changing availability, matching the real behavior (a location
   update never flips an assigned rider back to available).
   Base this on the ACTUAL state transitions in the code, not a generic
   guess — check main.go/scripts.go for what really happens.

**B. How the project started** — use the factual account above verbatim
in substance (you may lightly rephrase for flow but not change facts or
add invented interviewer dialogue). State plainly that the 30-minute figure
describes the intended level of human involvement, not a measured
duration.

**C. Requirements and design discussion** — Q&A format, each answer tagged
with its source using these exact labels: "User requirement", "Proposed
design (ChatGPT-generated spec)", "Implementation decision (build-time)",
or "Confirmed clarification". Cover at least: why Redis for this demo; why
SQLite was added and why that's a confirmed clarification, not a default
choice (include the real question/decision described above); could a
database alone support H3 lookups; why keep both H3 mapping directions; why
retain raw coordinates alongside H3 cells; how far candidate search
extends and why it's bounded; how nearest-candidate ranking works; what
happens when two requests race for the same rider; what happens on a
retried order request (same vs. conflicting payload); how movement updates
cell membership atomically; how completion safely releases a rider; why
drop location has no routing/ETA; what was excluded and why. For items
that are genuinely just Claude/Codex implementation choices (e.g. exact H3
library, response status codes, key naming), label them "Implementation
decision (build-time)" — do not claim they were requirements.

Add a clearly marked subsection "Reconstructed design rationale" for any
additional polished Q&A you write beyond the literal record, explicitly
stating these are explanatory summaries, not a verbatim conversation.

**D. Prompting and agent workflow** — a collapsible `<details>` section
containing the FULL original take-home prompt (the coordinator will give
you its exact text to paste verbatim — for now leave a clear placeholder
comment `<!-- COORDINATOR: insert full original prompt verbatim here -->`
inside the details block so it can be dropped in exactly). Then: this
documentation prompt's substance (your own instructions, summarized
honestly, in a collapsible section), Claude's responsibilities as
coordinator, the fact that there were two actual implementation/
verification Codex passes (describe them honestly — what each one was
asked to do and actually did, pulling from NOTES-impl.md / VERIFICATION.md
for what verification found/fixed), and the sandboxing fact from above.

**E. Implementation walkthrough** — project structure/file paths (actual
ones from the repo); endpoint contracts with real example requests/
responses (match the real JSON shapes in the code); the real Redis key
table (name pattern, type, purpose) straight from NOTES-impl.md; H3
resolution/neighborhood lookup explanation; distance ranking method
(haversine, as actually implemented); how the three Lua operations work;
order idempotency; configuration table; error handling/status codes
table (pull exact codes/messages from the code, e.g. `no_rider_available`,
`order_payload_conflict`, `route_not_found`, etc.).

**F. Verification and results** — a table: scenario | expected | method |
actual result, built directly from VERIFICATION.md, split into
passed / fixed-after-a-found-bug / could-not-run. Include the exact
commands that were actually run (from VERIFICATION.md). Do not add
anything VERIFICATION.md doesn't support.

**G. Run and review** — prerequisites; the one startup command (`docker
compose up --build`, noted as written-but-not-executed-in-build-env, plus
the manual local alternative that WAS actually run: `go run .` /
`go build` + local redis-server); full curl walkthrough; test commands;
a five-minute code-reading path (which files/functions to read in what
order to understand the whole system); known limitations; brief future
considerations (durable rehydration of Redis from SQLite on restart, stale
rider expiry) explicitly NOT implemented.

### Style notes
Avoid filler and repetition across sections — if a fact is fully explained
in E, just reference it from F/G rather than re-explaining. Use real numbers
and real file/line references where it helps, but don't pad with vague
adjectives ("robust", "production-grade", "seamless") — this is an honest
small demo, say so plainly.

Do not `git commit`. Print the final list of files you created and a short
note on what you verified (that it opens as valid HTML — just eyeball the
tag structure since no browser is available; check for any unclosed tags or
obviously broken code-block escaping of `<`/`>`/`&` inside `<pre><code>`
blocks that would break rendering).
```

</details>

**Result:** Produced `README.md` and `docs/index.html`. The coordinator
(Claude) then inserted this prompt-history file's own content and the
original prompt verbatim where the draft had left a placeholder, since the
docs agent did not have this file's exact text yet when it ran.

## Note on the build environment

Codex's sandboxed execution mode (bubblewrap) does not work on the Linux
build machine used for passes 1–3, because that machine restricts
unprivileged user namespaces by default (an Ubuntu 24.04 setting). Codex
was run unsandboxed instead, after explicit confirmation, scoped only to
this project's directory — it never touched any other path on that
machine. This is an environment/tooling fact, not part of the interview
account in section 2 of the original prompt.

A mid-build clarification also changed scope from the original prompt: the
person said, "use sqlite as the db... only redis is not the correct way,"
which conflicted with the original spec's "Redis only" rule and the
interview backstory. Claude flagged the conflict and offered three options;
the person chose to keep Redis as the sole live/candidate-lookup/atomicity
engine and add SQLite purely as a durable write-through record for riders
and assignments. This is reflected throughout the implementation and in the
design-discussion section of `docs/index.html`, labeled "Confirmed
clarification."
