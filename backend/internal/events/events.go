// Package events defines the Kafka envelope for ticket events (SRS 8.2), shared by the API and notifier.
package events

import (
	"time"

	"plms/internal/domain"
)

const (
	Version  = 1
	Producer = "plms-api"
)

type Envelope struct {
	EventID      string     `json:"eventId"`
	EventType    string     `json:"eventType"` // VehicleEntered | VehicleExited
	EventVersion int        `json:"eventVersion"`
	OccurredAt   time.Time  `json:"occurredAt"`
	Producer     string     `json:"producer"`
	Source       string     `json:"source"` // GATE | SIMULATOR | ADMIN
	Simulated    bool       `json:"simulated"`
	TraceID      string     `json:"traceId"`
	Data         TicketData `json:"data"`
}

type VehicleRef struct {
	ID            string             `json:"id"`
	VehicleNumber string             `json:"vehicleNumber"`
	VehicleType   domain.VehicleType `json:"vehicleType"`
	OwnerEmail    string             `json:"ownerEmail"`
}

type SpotRef struct {
	ID       string          `json:"id"`
	Code     string          `json:"code"`
	SpotType domain.SpotType `json:"spotType"`
	Floor    int             `json:"floor"`
}

// TicketData is the payload for both events. Exit fields are set on VehicleExited only;
// they are optional, so adding them keeps eventVersion 1.
type TicketData struct {
	TicketID     string      `json:"ticketId"`
	TicketNumber string      `json:"ticketNumber"`
	Vehicle      VehicleRef  `json:"vehicle"`
	Spot         SpotRef     `json:"spot"`
	EntryTime    time.Time   `json:"entryTime"`
	Rate         domain.Rate `json:"rate"`

	ExitTime        *time.Time `json:"exitTime,omitempty"`
	DurationMinutes *int       `json:"durationMinutes,omitempty"`
	BillableHours   *int       `json:"billableHours,omitempty"`
	FeePaise        *int64     `json:"feePaise,omitempty"`
	Currency        string     `json:"currency,omitempty"`
	PaidPaise       *int64     `json:"paidPaise,omitempty"`
	PaymentMethod   string     `json:"paymentMethod,omitempty"`
	PaidAt          *time.Time `json:"paidAt,omitempty"`
}
