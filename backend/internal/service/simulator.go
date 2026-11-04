package service

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/apperr"
	"plms/internal/domain"
	"plms/internal/platform/db"
	"plms/internal/store"
)

type LayoutVehicle struct {
	VehicleID     string             `json:"vehicleId"`
	VehicleNumber string             `json:"vehicleNumber"`
	VehicleType   domain.VehicleType `json:"vehicleType"`
	ColorSeed     string             `json:"colorSeed"`
	OwnerEmail    string             `json:"ownerEmail"`
	TicketID      string             `json:"ticketId"`
	TicketNumber  string             `json:"ticketNumber"`
	TicketStatus  string             `json:"ticketStatus"`
	EntryTime     time.Time          `json:"entryTime"`
	IsSimulated   bool               `json:"isSimulated"`
	Rate          domain.Rate        `json:"rate"`
}

type LayoutSpot struct {
	ID       string          `json:"id"`
	Code     string          `json:"code"`
	SpotType domain.SpotType `json:"spotType"`
	Letter   string          `json:"letter"`
	Position int             `json:"position"`
	Status   string          `json:"status"`
	Vehicle  *LayoutVehicle  `json:"vehicle"`
}

type LayoutRow struct {
	Floor    int          `json:"floor"`
	RowLabel string       `json:"rowLabel"`
	Spots    []LayoutSpot `json:"spots"`
}

type LayoutGate struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type Layout struct {
	AsOf      time.Time    `json:"asOf"`
	FreeCount int          `json:"freeCount"`
	Floors    []int        `json:"floors"`
	Rows      []LayoutRow  `json:"rows"`
	Gates     []LayoutGate `json:"gates"`
}

// Layout returns spots grouped by floor and row with their parked vehicles (FR-41, FR-49, SRS 7.6).
func (s *Service) Layout(ctx context.Context) (Layout, error) {
	spots, err := store.ListSpots(ctx, s.Pool)
	if err != nil {
		return Layout{}, err
	}
	open, err := store.OpenTicketsBySpot(ctx, s.Pool)
	if err != nil {
		return Layout{}, err
	}
	gates, err := store.ListGates(ctx, s.Pool)
	if err != nil {
		return Layout{}, err
	}
	out := Layout{AsOf: time.Now().UTC(), Rows: []LayoutRow{}, Floors: []int{}, Gates: []LayoutGate{}}
	rowIdx := map[[2]any]int{}
	floors := map[int]bool{}
	for _, sp := range spots {
		if sp.Status == domain.SpotAvailable {
			out.FreeCount++
		}
		floors[sp.Floor] = true
		key := [2]any{sp.Floor, sp.RowLabel}
		i, ok := rowIdx[key]
		if !ok {
			i = len(out.Rows)
			rowIdx[key] = i
			out.Rows = append(out.Rows, LayoutRow{Floor: sp.Floor, RowLabel: sp.RowLabel, Spots: []LayoutSpot{}})
		}
		ls := LayoutSpot{ID: sp.ID, Code: sp.Code, SpotType: sp.SpotType, Letter: sp.SpotType.Letter(), Position: sp.Position, Status: sp.Status}
		if t, ok := open[sp.ID]; ok {
			seed := t.VehicleID
			if len(seed) > 8 {
				seed = seed[:8]
			}
			ls.Vehicle = &LayoutVehicle{VehicleID: t.VehicleID, VehicleNumber: t.VehicleNumber, VehicleType: t.VehicleType,
				ColorSeed: seed, OwnerEmail: t.OwnerEmail, TicketID: t.ID, TicketNumber: t.TicketNumber,
				TicketStatus: t.Status, EntryTime: t.EntryTime, IsSimulated: t.IsSimulated, Rate: t.Rate}
		}
		out.Rows[i].Spots = append(out.Rows[i].Spots, ls)
	}
	for f := range floors {
		out.Floors = append(out.Floors, f)
	}
	sort.Ints(out.Floors)
	for _, g := range gates {
		if g.IsSimulator && g.Active {
			out.Gates = append(out.Gates, LayoutGate{Name: g.Name, Kind: g.Kind})
		}
	}
	return out, nil
}

func (s *Service) SimulatorEvents(ctx context.Context, limit int) ([]store.EventSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return store.RecentEvents(ctx, s.Pool, limit, "")
}

// ResetSimulator cancels open simulator tickets and frees their spots (FR-48). No exit events
// are written, so no emails go out for reset vehicles.
func (s *Service) ResetSimulator(ctx context.Context, adminID string) (int, error) {
	if !s.Cfg.SimulatorEnabled {
		return 0, apperr.New(403, "FORBIDDEN", "The simulator is turned off on this server.")
	}
	n := 0
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		list, err := store.OpenSimulatedTickets(ctx, tx)
		if err != nil {
			return err
		}
		for _, t := range list {
			if err := store.CancelTicket(ctx, tx, t.ID); err != nil {
				return err
			}
			if err := store.SetSpotStatus(ctx, tx, t.SpotID, domain.SpotAvailable); err != nil {
				return err
			}
		}
		n = len(list)
		return s.audit(ctx, tx, adminID, "SIMULATOR_RESET", "TICKET", "", "", map[string]any{"cancelled": n})
	})
	return n, err
}
