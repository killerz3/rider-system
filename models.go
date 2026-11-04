package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

type point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
type inputPoint struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func (p inputPoint) validate() (point, error) {
	if p.Latitude == nil || p.Longitude == nil {
		return point{}, fmt.Errorf("latitude and longitude are required")
	}
	a, b := *p.Latitude, *p.Longitude
	if math.IsNaN(a) || math.IsInf(a, 0) || math.IsNaN(b) || math.IsInf(b, 0) || a < -90 || a > 90 || b < -180 || b > 180 {
		return point{}, fmt.Errorf("latitude must be -90..90 and longitude -180..180")
	}
	if a == 0 {
		a = 0
	}
	if b == 0 {
		b = 0
	}
	return point{a, b}, nil
}

type rider struct {
	ID                  string  `json:"id"`
	Latitude            float64 `json:"latitude"`
	Longitude           float64 `json:"longitude"`
	H3Cell              string  `json:"h3_cell"`
	Available           bool    `json:"available"`
	CurrentAssignmentID *string `json:"current_assignment_id"`
	UpdatedAt           string  `json:"updated_at"`
}
type assignment struct {
	ID          string  `json:"id"`
	OrderID     string  `json:"order_id"`
	RiderID     string  `json:"rider_id"`
	Pickup      point   `json:"pickup"`
	Drop        point   `json:"drop"`
	PickupCell  string  `json:"pickup_h3_cell"`
	DropCell    string  `json:"drop_h3_cell"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt *string `json:"completed_at"`
}

func riderFrom(m map[string]string) rider {
	a, _ := strconv.ParseFloat(m["latitude"], 64)
	b, _ := strconv.ParseFloat(m["longitude"], 64)
	r := rider{ID: m["id"], Latitude: a, Longitude: b, H3Cell: m["h3_cell"], Available: m["available"] == "1", UpdatedAt: m["updated_at"]}
	if s := m["current_assignment_id"]; s != "" {
		r.CurrentAssignmentID = &s
	}
	return r
}
func assignmentFrom(m map[string]string) assignment {
	var a assignment
	_ = json.Unmarshal([]byte(m["record"]), &a)
	a.Status = m["status"]
	if s := m["completed_at"]; s != "" {
		a.CompletedAt = &s
	}
	return a
}
func distance(a, b point) float64 {
	r := math.Pi / 180
	x := (b.Latitude - a.Latitude) * r
	y := (b.Longitude - a.Longitude) * r
	h := math.Sin(x/2)*math.Sin(x/2) + math.Cos(a.Latitude*r)*math.Cos(b.Latitude*r)*math.Sin(y/2)*math.Sin(y/2)
	return 2 * 6371000 * math.Asin(math.Sqrt(math.Min(1, h)))
}
