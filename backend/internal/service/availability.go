package service

import (
	"context"
	"time"

	"plms/internal/domain"
	"plms/internal/store"
)

type FreeSpot struct {
	Code     string          `json:"code"`
	SpotType domain.SpotType `json:"spotType"`
	Floor    int             `json:"floor"`
	Overflow bool            `json:"overflow"`
}

type Panel struct {
	VehicleType       domain.VehicleType `json:"vehicleType"`
	Label             string             `json:"label"`
	SpotType          domain.SpotType    `json:"spotType"`
	Available         int                `json:"available"`
	Total             int                `json:"total"`
	Occupied          int                `json:"occupied"`
	OutOfService      int                `json:"outOfService"`
	OverflowAvailable int                `json:"overflowAvailable"`
	Full              bool               `json:"full"`
	FreeSpots         []FreeSpot         `json:"freeSpots"`
}

type Availability struct {
	AsOf            time.Time `json:"asOf"`
	OverflowEnabled bool      `json:"overflowEnabled"`
	Panels          []Panel   `json:"panels"`
}

var panelLabels = map[domain.VehicleType]string{
	domain.Motorcycle: "Motorcycles", domain.Car: "Cars", domain.Truck: "Trucks",
}

// Availability builds the three public panels (FR-10 to FR-13).
func (s *Service) Availability(ctx context.Context) (Availability, error) {
	spots, err := store.ListSpots(ctx, s.Pool)
	if err != nil {
		return Availability{}, err
	}
	overflow, err := store.OverflowEnabled(ctx, s.Pool)
	if err != nil {
		return Availability{}, err
	}
	out := Availability{AsOf: time.Now().UTC(), OverflowEnabled: overflow}
	for _, vt := range domain.VehicleTypes {
		home := domain.HomeSpot(vt)
		p := Panel{VehicleType: vt, Label: panelLabels[vt], SpotType: home, FreeSpots: []FreeSpot{}}
		allowed := domain.AllowedSpots(vt, overflow)
		for _, sp := range spots {
			if sp.SpotType == home {
				switch sp.Status {
				case domain.SpotAvailable:
					p.Available++
				case domain.SpotOccupied:
					p.Occupied++
				case domain.SpotOutOfService:
					p.OutOfService++
				}
				if sp.Status != domain.SpotOutOfService {
					p.Total++
				}
			}
			if sp.Status != domain.SpotAvailable {
				continue
			}
			for i, st := range allowed {
				if st == sp.SpotType {
					p.FreeSpots = append(p.FreeSpots, FreeSpot{Code: sp.Code, SpotType: sp.SpotType, Floor: sp.Floor, Overflow: i > 0})
					if i > 0 {
						p.OverflowAvailable++
					}
				}
			}
		}
		p.Full = p.Available+p.OverflowAvailable == 0
		out.Panels = append(out.Panels, p)
	}
	return out, nil
}
