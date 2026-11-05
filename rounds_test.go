package main

import (
	"context"
	"testing"

	h3 "github.com/uber/h3-go/v4"
)

func center(t *testing.T, c h3.Cell) point {
	t.Helper()
	ll, e := h3.CellToLatLng(c)
	if e != nil {
		t.Fatal(e)
	}
	return point{ll.Lat, ll.Lng}
}

// lerp returns a + f*(b-a); fine for points a few kilometres apart.
func lerp(a, b point, f float64) point {
	return point{a.Latitude + f*(b.Latitude-a.Latitude), a.Longitude + f*(b.Longitude-a.Longitude)}
}

func cellOf(t *testing.T, p point) h3.Cell {
	t.Helper()
	c, e := h3.LatLngToCell(h3.NewLatLng(p.Latitude, p.Longitude), 7)
	if e != nil {
		t.Fatal(e)
	}
	return c
}

// pickupRings returns the pickup cell's rings out to distance 3.
func pickupRings(t *testing.T) [][]h3.Cell {
	t.Helper()
	rings, e := h3.GridDiskDistances(cellOf(t, pickup), 3)
	if e != nil {
		t.Fatal(e)
	}
	return rings
}

func TestSearchStopsOneRingAfterFirstHit(t *testing.T) {
	s, h := fixture(t)
	register(t, h, "near", pickup)
	rings := pickupRings(t)[:3]
	for _, tc := range []struct {
		mode string
		last int
	}{{"rounds", 1}, {"disk", 2}} {
		t.Run(tc.mode, func(t *testing.T) {
			s.cfg.SearchMode = tc.mode
			found, last, e := s.search(context.Background(), rings, 0, pickup)
			if e != nil {
				t.Fatal(e)
			}
			if last != tc.last || len(found) != 1 || found[0].id != "near" {
				t.Fatalf("last=%d found=%v", last, found)
			}
		})
	}
}

func TestRoundsReachOuterRing(t *testing.T) {
	rings := pickupRings(t)
	for _, mode := range []string{"rounds", "disk"} {
		t.Run(mode, func(t *testing.T) {
			s, h := fixture(t)
			s.cfg.SearchMode = mode
			register(t, h, "ring3", center(t, rings[3][0]))
			var v map[string]string
			expect(t, request(h, "POST", "/assignments", order("too-far", pickup, dropoff)), 409, &v)
			register(t, h, "ring2", center(t, rings[2][0]))
			if a := create(t, h, "ring2-order"); a.RiderID != "ring2" {
				t.Fatal(a)
			}
		})
	}
}

// A rider just across the pickup cell's edge is closer than one at the far side of the pickup cell,
// so the one-ring lookahead must pick it even though round 0 already found someone.
func TestRoundsPreferNearerRiderInNextRing(t *testing.T) {
	rings := pickupRings(t)
	c, n := center(t, rings[0][0]), center(t, rings[1][0])
	p := lerp(c, n, 0.45)
	nearby, farSide := lerp(c, n, 0.55), lerp(c, n, -0.4)
	if cellOf(t, p) != rings[0][0] || cellOf(t, farSide) != rings[0][0] || cellOf(t, nearby) != rings[1][0] {
		t.Fatal("test geometry does not land in the expected cells")
	}
	for _, mode := range []string{"rounds", "disk"} {
		t.Run(mode, func(t *testing.T) {
			s, h := fixture(t)
			s.cfg.SearchMode = mode
			register(t, h, "far-side", farSide)
			register(t, h, "nearby", nearby)
			var a assignment
			expect(t, request(h, "POST", "/assignments", order("edge", p, dropoff)), 201, &a)
			if a.RiderID != "nearby" {
				t.Fatal(a)
			}
		})
	}
}
