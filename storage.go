package main

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
)

func openDB(path string) (*sql.DB, error) {
	d, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	d.SetMaxOpenConns(1)
	_, e = d.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=3000;
CREATE TABLE IF NOT EXISTS riders(id TEXT PRIMARY KEY,lat REAL NOT NULL,lon REAL NOT NULL,h3_cell TEXT NOT NULL,available INTEGER NOT NULL,current_assignment_id TEXT,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS assignments(id TEXT PRIMARY KEY,order_id TEXT NOT NULL UNIQUE,rider_id TEXT NOT NULL,pickup_lat REAL NOT NULL,pickup_lon REAL NOT NULL,pickup_cell TEXT NOT NULL,drop_lat REAL NOT NULL,drop_lon REAL NOT NULL,drop_cell TEXT NOT NULL,status TEXT NOT NULL,created_at TEXT NOT NULL,completed_at TEXT);`)
	if e != nil {
		d.Close()
		return nil, e
	}
	return d, nil
}
func (s *server) persist(ctx context.Context, r rider, a *assignment) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO riders VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET lat=excluded.lat,lon=excluded.lon,h3_cell=excluded.h3_cell,available=excluded.available,current_assignment_id=excluded.current_assignment_id,updated_at=excluded.updated_at`, r.ID, r.Latitude, r.Longitude, r.H3Cell, r.Available, r.CurrentAssignmentID, r.UpdatedAt)
	if e != nil {
		return e
	}
	if a != nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO assignments VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,completed_at=excluded.completed_at`, a.ID, a.OrderID, a.RiderID, a.Pickup.Latitude, a.Pickup.Longitude, a.PickupCell, a.Drop.Latitude, a.Drop.Longitude, a.DropCell, a.Status, a.CreatedAt, a.CompletedAt)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
