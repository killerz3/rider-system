package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	h3 "github.com/uber/h3-go/v4"
)

type server struct {
	cfg   config
	redis *redis.Client
	db    *sql.DB
	mu    sync.Mutex
}

func main() {
	c, e := loadConfig()
	if e != nil {
		log.Fatal(e)
	}
	d, e := openDB(c.SQLitePath)
	if e != nil {
		log.Fatal(e)
	}
	defer d.Close()
	rc := redis.NewClient(&redis.Options{Addr: c.RedisAddr, DialTimeout: operationTimeout, ReadTimeout: operationTimeout, WriteTimeout: operationTimeout, ContextTimeoutEnabled: true})
	defer rc.Close()
	s := &server{cfg: c, redis: rc, db: d}
	m := http.NewServeMux()
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotFound, "route_not_found", "route or method does not exist")
	})
	m.HandleFunc("GET /health", s.health)
	m.HandleFunc("PUT /riders/{rider_id}/location", s.location)
	m.HandleFunc("POST /assignments", s.assign)
	m.HandleFunc("GET /assignments/{assignment_id}", s.getAssignment)
	m.HandleFunc("POST /assignments/{assignment_id}/complete", s.complete)
	h := &http.Server{Addr: ":" + c.HTTPPort, Handler: m, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Shutdown(c)
	}()
	log.Printf("listening on %s", h.Addr)
	if e = h.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	respond(w, status, map[string]string{"code": code, "message": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		fail(w, 400, "invalid_json", e.Error())
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		fail(w, 400, "invalid_json", "expected exactly one JSON object")
		return false
	}
	return true
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func (s *server) cell(p point) (string, error) {
	c, e := h3.LatLngToCell(h3.NewLatLng(p.Latitude, p.Longitude), s.cfg.H3Resolution)
	return c.String(), e
}
func (s *server) readRider(ctx context.Context, id string) (rider, error) {
	m, e := s.redis.HGetAll(ctx, prefix+"rider:"+id).Result()
	if e != nil {
		return rider{}, e
	}
	if len(m) == 0 {
		return rider{}, redis.Nil
	}
	return riderFrom(m), nil
}
func (s *server) readAssignment(ctx context.Context, id string) (assignment, error) {
	m, e := s.redis.HGetAll(ctx, prefix+"assignment:"+id).Result()
	if e != nil {
		return assignment{}, e
	}
	if len(m) == 0 {
		return assignment{}, redis.Nil
	}
	return assignmentFrom(m), nil
}
func backend(w http.ResponseWriter, e error) {
	log.Printf("storage failure: %v", e)
	fail(w, 503, "storage_unavailable", "storage operation failed; Redis may already contain the change, retry the request")
}
func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, c := context.WithTimeout(r.Context(), operationTimeout)
	defer c()
	if e := s.redis.Ping(ctx).Err(); e != nil {
		respond(w, 503, map[string]string{"status": "degraded"})
		return
	}
	respond(w, 200, map[string]string{"status": "ok"})
}
func (s *server) location(w http.ResponseWriter, r *http.Request) {
	var in inputPoint
	if !decode(w, r, &in) {
		return
	}
	p, e := in.validate()
	if e != nil {
		fail(w, 400, "invalid_coordinates", e.Error())
		return
	}
	cell, e := s.cell(p)
	if e != nil {
		fail(w, 400, "invalid_coordinates", e.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, c := context.WithTimeout(r.Context(), operationTimeout)
	defer c()
	id := r.PathValue("rider_id")
	_, e = locationScript.Run(ctx, s.redis, []string{prefix + "rider:" + id}, prefix, id, cell, p.Latitude, p.Longitude, now()).Result()
	if e != nil {
		backend(w, e)
		return
	}
	v, e := s.readRider(ctx, id)
	if e == nil {
		e = s.persist(ctx, v, nil)
	}
	if e != nil {
		backend(w, e)
		return
	}
	respond(w, 200, v)
}
func (s *server) getAssignment(w http.ResponseWriter, r *http.Request) {
	ctx, c := context.WithTimeout(r.Context(), operationTimeout)
	defer c()
	a, e := s.readAssignment(ctx, r.PathValue("assignment_id"))
	if errors.Is(e, redis.Nil) {
		fail(w, 404, "assignment_not_found", "assignment does not exist")
		return
	}
	if e != nil {
		backend(w, e)
		return
	}
	respond(w, 200, a)
}
func (s *server) finishResponse(ctx context.Context, w http.ResponseWriter, id string, status int) {
	a, e := s.readAssignment(ctx, id)
	if e != nil {
		backend(w, e)
		return
	}
	v, e := s.readRider(ctx, a.RiderID)
	if e == nil {
		e = s.persist(ctx, v, &a)
	}
	if e != nil {
		backend(w, e)
		return
	}
	respond(w, status, a)
}
func (s *server) assign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrderID string     `json:"order_id"`
		Pickup  inputPoint `json:"pickup"`
		Drop    inputPoint `json:"drop"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.OrderID == "" {
		fail(w, 400, "invalid_order_id", "order_id is required")
		return
	}
	p, e := in.Pickup.validate()
	if e != nil {
		fail(w, 400, "invalid_pickup", e.Error())
		return
	}
	d, e := in.Drop.validate()
	if e != nil {
		fail(w, 400, "invalid_drop", e.Error())
		return
	}
	pc, e := s.cell(p)
	if e != nil {
		fail(w, 400, "invalid_pickup", e.Error())
		return
	}
	dc, e := s.cell(d)
	if e != nil {
		fail(w, 400, "invalid_drop", e.Error())
		return
	}
	payloadBytes, _ := json.Marshal(struct{ Pickup, Drop point }{p, d})
	payload := string(payloadBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), operationTimeout)
	defer cancel()
	orderKey := prefix + "order:" + in.OrderID
	existing, e := s.redis.Get(ctx, orderKey).Result()
	if e == nil {
		stored, err := s.redis.HGet(ctx, prefix+"assignment:"+existing, "payload").Result()
		if err != nil {
			backend(w, err)
			return
		}
		if stored != payload {
			fail(w, 409, "order_payload_conflict", "order already has an assignment with different coordinates")
			return
		}
		s.finishResponse(ctx, w, existing, 200)
		return
	}
	if !errors.Is(e, redis.Nil) {
		backend(w, e)
		return
	}
	pickupCell, e := h3.LatLngToCell(h3.NewLatLng(p.Latitude, p.Longitude), s.cfg.H3Resolution)
	if e != nil {
		backend(w, e)
		return
	}
	cells, e := h3.GridDisk(pickupCell, s.cfg.GridDistance)
	if e != nil {
		backend(w, e)
		return
	}
	keys := make([]string, 0, len(cells))
	for _, cell := range cells {
		keys = append(keys, prefix+"cell:"+cell.String())
	}
	ids, e := s.redis.SUnion(ctx, keys...).Result()
	if e != nil {
		backend(w, e)
		return
	}
	type candidate struct {
		id       string
		distance float64
	}
	candidates := []candidate{}
	for _, id := range ids {
		v, err := s.readRider(ctx, id)
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			backend(w, err)
			return
		}
		if v.Available {
			candidates = append(candidates, candidate{id, distance(point{v.Latitude, v.Longitude}, p)})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance == candidates[j].distance {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].distance < candidates[j].distance
	})
	b := make([]byte, 16)
	if _, e = rand.Read(b); e != nil {
		backend(w, e)
		return
	}
	id := hex.EncodeToString(b)
	// An empty candidate still runs the order check to catch concurrent duplicates with no candidates.
	candidates = append(candidates, candidate{id: ""})
	for _, candidate := range candidates {
		a := assignment{ID: id, OrderID: in.OrderID, RiderID: candidate.id, Pickup: p, Drop: d, PickupCell: pc, DropCell: dc, Status: "active", CreatedAt: now()}
		record, _ := json.Marshal(a)
		riderKey := prefix + "rider:" + candidate.id
		if candidate.id == "" {
			riderKey = prefix + "no-candidate:" + id
		}
		result, err := claimScript.Run(ctx, s.redis, []string{orderKey, riderKey, prefix + "assignment:" + id}, prefix, payload, id, string(record), a.CreatedAt, candidate.id).StringSlice()
		if err != nil {
			backend(w, err)
			return
		}
		switch result[0] {
		case "conflict":
			fail(w, 409, "order_payload_conflict", "order already has an assignment with different coordinates")
			return
		case "existing":
			s.finishResponse(ctx, w, result[1], 200)
			return
		case "claimed":
			s.finishResponse(ctx, w, result[1], 201)
			return
		}
	}
	fail(w, 409, "no_rider_available", "no available rider in the pickup search area")
}
func (s *server) complete(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, c := context.WithTimeout(r.Context(), operationTimeout)
	defer c()
	id := r.PathValue("assignment_id")
	result, e := completeScript.Run(ctx, s.redis, []string{prefix + "assignment:" + id}, prefix, now(), id).StringSlice()
	if e != nil {
		backend(w, e)
		return
	}
	if result[0] == "missing" {
		fail(w, 404, "assignment_not_found", "assignment does not exist")
		return
	}
	s.finishResponse(ctx, w, id, 200)
}
