package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
	h3 "github.com/uber/h3-go/v4"
)

// Each test uses Redis database 15 and refuses to run over existing dispatch data.
// No FLUSHDB is used; cleanup removes only keys created in the initially empty namespace.
func fixture(t *testing.T) (*server, http.Handler) {
	t.Helper()
	rc := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "127.0.0.1:6379"), DB: 15})
	ctx := context.Background()
	if err := rc.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	keys, err := rc.Keys(ctx, prefix+"*").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatal("Redis DB 15 dispatch namespace must be empty")
	}
	dir, err := os.MkdirTemp(".", "verification-db-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	db, err := openDB(filepath.Join(dir, "verify.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := &server{cfg: config{H3Resolution: 7, GridDistance: 2}, redis: rc, db: db}
	t.Cleanup(func() {
		keys, e := rc.Keys(ctx, prefix+"*").Result()
		if e != nil {
			t.Error(e)
		} else if len(keys) > 0 {
			if e = rc.Del(ctx, keys...).Err(); e != nil {
				t.Error(e)
			}
		}
		db.Close()
		rc.Close()
	})
	m := http.NewServeMux()
	m.HandleFunc("PUT /riders/{rider_id}/location", s.location)
	m.HandleFunc("POST /assignments", s.assign)
	m.HandleFunc("GET /assignments/{assignment_id}", s.getAssignment)
	m.HandleFunc("POST /assignments/{assignment_id}/complete", s.complete)
	m.HandleFunc("GET /health", s.health)
	return s, m
}
func request(h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var b []byte
	if raw, ok := body.(string); ok {
		b = []byte(raw)
	} else if body != nil {
		b, _ = json.Marshal(body)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(b)))
	return w
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int, out any) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d want %d: %s", w.Code, status, w.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
}

var pickup = point{37.775, -122.418}
var dropoff = point{37.8, -122.45}

func order(id string, p, d point) any { return map[string]any{"order_id": id, "pickup": p, "drop": d} }
func register(t *testing.T, h http.Handler, id string, p point) rider {
	t.Helper()
	var r rider
	expect(t, request(h, "PUT", "/riders/"+id+"/location", p), 200, &r)
	return r
}
func create(t *testing.T, h http.Handler, id string) assignment {
	t.Helper()
	var a assignment
	expect(t, request(h, "POST", "/assignments", order(id, pickup, dropoff)), 201, &a)
	return a
}
func readR(t *testing.T, s *server, id string) rider {
	t.Helper()
	r, e := s.readRider(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestLocationAndLifecycle(t *testing.T) {
	s, h := fixture(t)
	ctx := context.Background()
	r := register(t, h, "r1", pickup)
	if !r.Available || r.CurrentAssignmentID != nil {
		t.Fatal(r)
	}
	cell, e := h3.LatLngToCell(h3.NewLatLng(pickup.Latitude, pickup.Longitude), 7)
	if e != nil {
		t.Fatal(e)
	}
	if r.H3Cell != cell.String() {
		t.Fatal("incorrect location H3")
	}
	center, e := h3.CellToLatLng(cell)
	if e != nil {
		t.Fatal(e)
	}
	r = register(t, h, "r1", point{center.Lat, center.Lng})
	if r.H3Cell != cell.String() {
		t.Fatal("same cell changed")
	}
	disk, e := h3.GridDisk(cell, 1)
	if e != nil {
		t.Fatal(e)
	}
	var neighbor h3.Cell
	for _, c := range disk {
		if c != cell {
			neighbor = c
			break
		}
	}
	ll, e := h3.CellToLatLng(neighbor)
	if e != nil {
		t.Fatal(e)
	}
	r = register(t, h, "r1", point{ll.Lat, ll.Lng})
	if r.H3Cell != neighbor.String() {
		t.Fatal("boundary not crossed")
	}
	member, e := s.redis.SIsMember(ctx, prefix+"cell:"+cell.String(), "r1").Result()
	if e != nil || member {
		t.Fatalf("old membership %v %v", member, e)
	}
	member, e = s.redis.SIsMember(ctx, prefix+"cell:"+neighbor.String(), "r1").Result()
	if e != nil || !member {
		t.Fatalf("new membership %v %v", member, e)
	}
	a := create(t, h, "o1")
	if a.RiderID != "r1" || a.PickupCell != cell.String() {
		t.Fatal(a)
	}
	dc, e := h3.LatLngToCell(h3.NewLatLng(dropoff.Latitude, dropoff.Longitude), 7)
	if e != nil {
		t.Fatal(e)
	}
	if a.DropCell != dc.String() || a.Pickup != pickup || a.Drop != dropoff {
		t.Fatal("pickup/drop conversion", a)
	}
	r = register(t, h, "r1", pickup)
	if r.Available || r.CurrentAssignmentID == nil || *r.CurrentAssignmentID != a.ID {
		t.Fatal("location freed assigned rider", r)
	}
	var status string
	var available int
	var current string
	if e = s.db.QueryRow("SELECT status FROM assignments WHERE id=?", a.ID).Scan(&status); e != nil || status != "active" {
		t.Fatalf("SQLite active: %s %v", status, e)
	}
	if e = s.db.QueryRow("SELECT available,current_assignment_id FROM riders WHERE id='r1'").Scan(&available, &current); e != nil || available != 0 || current != a.ID {
		t.Fatalf("SQLite rider %d %s %v", available, current, e)
	}
	for _, change := range []struct {
		name string
		p, d point
	}{{"pickup", dropoff, dropoff}, {"drop", pickup, pickup}} {
		t.Run(change.name, func(t *testing.T) {
			var v map[string]string
			expect(t, request(h, "POST", "/assignments", order("o1", change.p, change.d)), 409, &v)
			if v["code"] != "order_payload_conflict" {
				t.Fatal(v)
			}
		})
	}
	var got assignment
	expect(t, request(h, "GET", "/assignments/"+a.ID, nil), 200, &got)
	if got.ID != a.ID {
		t.Fatal(got)
	}
	expect(t, request(h, "POST", "/assignments/"+a.ID+"/complete", nil), 200, &got)
	if got.Status != "completed" || got.CompletedAt == nil {
		t.Fatal(got)
	}
	r = readR(t, s, "r1")
	if !r.Available || r.CurrentAssignmentID != nil {
		t.Fatal(r)
	}
	var completed string
	if e = s.db.QueryRow("SELECT status,completed_at FROM assignments WHERE id=?", a.ID).Scan(&status, &completed); e != nil || status != "completed" || completed != *got.CompletedAt {
		t.Fatalf("SQLite complete %s %s %v", status, completed, e)
	}
	if e = s.db.QueryRow("SELECT available FROM riders WHERE id='r1'").Scan(&available); e != nil || available != 1 {
		t.Fatalf("SQLite available %d %v", available, e)
	}
	firstCompletion := *got.CompletedAt
	expect(t, request(h, "POST", "/assignments/"+a.ID+"/complete", nil), 200, &got)
	if *got.CompletedAt != firstCompletion {
		t.Fatal("completion timestamp changed")
	}
	b := create(t, h, "o2")
	before := readR(t, s, "r1")
	expect(t, request(h, "POST", "/assignments/"+a.ID+"/complete", nil), 200, nil)
	r = readR(t, s, "r1")
	if r.Available || r.CurrentAssignmentID == nil || *r.CurrentAssignmentID != b.ID || r.UpdatedAt != before.UpdatedAt {
		t.Fatal("old completion changed newer claim", r)
	}
	expect(t, request(h, "GET", "/health", nil), 200, nil)
}
func TestNoRiderAvailable(t *testing.T) {
	_, h := fixture(t)
	for _, scenario := range []string{"empty", "far", "busy"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "far" {
				register(t, h, "far", point{-33.86, 151.2})
			}
			if scenario == "busy" {
				register(t, h, "near", pickup)
				create(t, h, "claimed")
			}
			var v map[string]string
			expect(t, request(h, "POST", "/assignments", order(scenario, pickup, dropoff)), 409, &v)
			if v["code"] != "no_rider_available" {
				t.Fatal(v)
			}
		})
	}
}

// Independent server objects deliberately bypass the process-local mutex, so
// simultaneous HTTP requests exercise Redis Lua as the actual claim authority.
func TestConcurrentClaims(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprint("identical_order=", same), func(t *testing.T) {
			s, h := fixture(t)
			nRiders := 3
			if same {
				nRiders = 1
			}
			for i := 0; i < nRiders; i++ {
				register(t, h, fmt.Sprint("r", i), pickup)
			}
			const n = 24
			responses := make(chan *httptest.ResponseRecorder, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					other := &server{cfg: s.cfg, redis: s.redis, db: s.db}
					m := http.NewServeMux()
					m.HandleFunc("POST /assignments", other.assign)
					id := fmt.Sprint("order", i)
					if same {
						id = "same"
					}
					<-start
					responses <- request(m, "POST", "/assignments", order(id, pickup, dropoff))
				}(i)
			}
			close(start)
			wg.Wait()
			close(responses)
			winners := map[string]string{}
			ids := map[string]bool{}
			created := 0
			success := 0
			for w := range responses {
				if w.Code == 201 || w.Code == 200 {
					var a assignment
					expect(t, w, w.Code, &a)
					success++
					if w.Code == 201 {
						created++
					}
					if old, ok := winners[a.RiderID]; ok && old != a.ID {
						t.Fatal("double assignment", old, a)
					}
					winners[a.RiderID] = a.ID
					ids[a.ID] = true
				} else {
					var v map[string]string
					expect(t, w, 409, &v)
					if same || v["code"] != "no_rider_available" {
						t.Fatal(v)
					}
				}
			}
			if created != nRiders || len(ids) != nRiders {
				t.Fatalf("created=%d unique=%d want=%d", created, len(ids), nRiders)
			}
			if same && success != n {
				t.Fatalf("only %d retries succeeded", success)
			}
			for rid, id := range winners {
				r := readR(t, s, rid)
				if r.Available || r.CurrentAssignmentID == nil || *r.CurrentAssignmentID != id {
					t.Fatal(r)
				}
			}
		})
	}
}
func TestInvalidInputs(t *testing.T) {
	_, h := fixture(t)
	cases := []struct{ name, path, body string }{
		{"missing", "/riders/r/location", `{}`}, {"missing longitude", "/riders/r/location", `{"latitude":0}`}, {"null", "/riders/r/location", `{"latitude":null,"longitude":0}`}, {"latitude range", "/riders/r/location", `{"latitude":91,"longitude":0}`}, {"longitude range", "/riders/r/location", `{"latitude":0,"longitude":-181}`}, {"malformed", "/riders/r/location", `{`}, {"trailing", "/riders/r/location", `{} {}`}, {"wrong type", "/riders/r/location", `{"latitude":"zero","longitude":0}`}, {"unknown field", "/riders/r/location", `{"latitude":0,"longitude":0,"extra":1}`},
		{"missing order", "/assignments", `{"pickup":{"latitude":0,"longitude":0},"drop":{"latitude":0,"longitude":0}}`}, {"missing pickup", "/assignments", `{"order_id":"x","drop":{"latitude":0,"longitude":0}}`}, {"missing drop coordinate", "/assignments", `{"order_id":"x","pickup":{"latitude":0,"longitude":0},"drop":{"latitude":0}}`}, {"invalid drop", "/assignments", `{"order_id":"x","pickup":{"latitude":0,"longitude":0},"drop":{"latitude":-91,"longitude":0}}`}, {"invalid pickup", "/assignments", `{"order_id":"x","pickup":{"latitude":0,"longitude":181},"drop":{"latitude":0,"longitude":0}}`}, {"malformed assignment", "/assignments", `[`}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			method := "PUT"
			if c.path == "/assignments" {
				method = "POST"
			}
			var v map[string]string
			expect(t, request(h, method, c.path, c.body), 400, &v)
			if v["code"] == "" || v["message"] == "" {
				t.Fatal("unclear error", v)
			}
		})
	}
	register(t, h, "zero", point{0, 0})
	var a assignment
	expect(t, request(h, "POST", "/assignments", order("zero", point{0, 0}, point{0, 0})), 201, &a)
}
