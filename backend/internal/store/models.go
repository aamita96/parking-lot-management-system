// Package store is the repository layer: parameterised SQL over pgx (NFR-8, NFR-15).
// Every function takes a db.DBTX so it can run inside a service-level transaction.
package store

import (
	"time"

	"plms/internal/domain"
)

type Owner struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Mobile          *string    `json:"mobile"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type EmailCode struct {
	ID          string
	OwnerID     string
	Purpose     string
	CodeHash    string
	SentTo      string
	TargetEmail *string
	ExpiresAt   time.Time
	Attempts    int
	ConsumedAt  *time.Time
	CreatedAt   time.Time
}

type Vehicle struct {
	ID             string             `json:"id"`
	VehicleNumber  string             `json:"vehicleNumber"`
	VehicleType    domain.VehicleType `json:"vehicleType"`
	OwnerID        string             `json:"ownerId"`
	OwnerEmail     string             `json:"ownerEmail"`
	OwnerVerified  bool               `json:"-"`
	RegistrationID *string            `json:"registrationId"`
	Status         string             `json:"status"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

type Spot struct {
	ID       string          `json:"id"`
	Code     string          `json:"code"`
	SpotType domain.SpotType `json:"spotType"`
	Floor    int             `json:"floor"`
	RowLabel string          `json:"rowLabel"`
	Position int             `json:"position"`
	Status   string          `json:"status"`
	Version  int             `json:"version"`
}

type PricingRule struct {
	ID              string             `json:"id"`
	VehicleType     domain.VehicleType `json:"vehicleType"`
	HourlyRatePaise int64              `json:"hourlyRatePaise"`
	DailyCapPaise   int64              `json:"dailyCapPaise"`
	GraceMinutes    int                `json:"graceMinutes"`
	EffectiveFrom   time.Time          `json:"effectiveFrom"`
	EffectiveTo     *time.Time         `json:"effectiveTo"`
}

func (p PricingRule) Rate() domain.Rate {
	return domain.Rate{HourlyRatePaise: p.HourlyRatePaise, DailyCapPaise: p.DailyCapPaise, GraceMinutes: p.GraceMinutes}
}

type GateDevice struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	IsSimulator bool       `json:"isSimulator"`
	Active      bool       `json:"active"`
	HasKey      bool       `json:"hasKey"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type Ticket struct {
	ID              string             `json:"id"`
	TicketNumber    string             `json:"ticketNumber"`
	VehicleID       string             `json:"vehicleId"`
	SpotID          string             `json:"spotId"`
	PricingRuleID   string             `json:"pricingRuleId"`
	EntryGateID     *string            `json:"entryGateId"`
	ExitGateID      *string            `json:"exitGateId"`
	VehicleType     domain.VehicleType `json:"vehicleType"`
	SpotType        domain.SpotType    `json:"spotType"`
	OwnerEmail      string             `json:"ownerEmail"`
	EntryTime       time.Time          `json:"entryTime"`
	ExitTime        *time.Time         `json:"exitTime"`
	DurationMinutes *int               `json:"durationMinutes"`
	BillableHours   *int               `json:"billableHours"`
	FeePaise        *int64             `json:"feePaise"`
	PaidPaise       *int64             `json:"paidPaise"`
	PaymentMethod   *string            `json:"paymentMethod"`
	PaidAt          *time.Time         `json:"paidAt"`
	Status          string             `json:"status"`
	IsSimulated     bool               `json:"isSimulated"`
	Source          string             `json:"source"`

	// Joined for display.
	VehicleNumber string      `json:"vehicleNumber"`
	OwnerID       string      `json:"ownerId"`
	SpotCode      string      `json:"spotCode"`
	SpotFloor     int         `json:"spotFloor"`
	SpotRow       string      `json:"spotRow"`
	SpotPosition  int         `json:"spotPosition"`
	Rate          domain.Rate `json:"rate"`
}

func (t Ticket) Open() bool {
	return t.Status == domain.TicketActive || t.Status == domain.TicketPaymentPending
}

type Scan struct {
	ID              int64      `json:"id"`
	GateID          string     `json:"gateId"`
	GateName        string     `json:"gateName"`
	GateKind        string     `json:"gateKind"`
	PlateRaw        string     `json:"plateRaw"`
	PlateNormalised *string    `json:"plateNormalised"`
	Result          string     `json:"result"`
	RejectReason    *string    `json:"rejectReason"`
	TicketID        *string    `json:"ticketId"`
	ScannedAt       time.Time  `json:"scannedAt"`
	Deduplicated    bool       `json:"deduplicated"`
	ResolvedAt      *time.Time `json:"resolvedAt"`
	Resolution      *string    `json:"resolution"`
}

type OutboxEvent struct {
	ID          int64
	EventID     string
	EventType   string
	AggregateID string
	Payload     []byte
	CreatedAt   time.Time
	PublishedAt *time.Time
	Attempts    int
	LastError   *string
}

type EmailNotification struct {
	ID                string     `json:"id"`
	EventID           string     `json:"eventId"`
	TicketID          string     `json:"ticketId"`
	Kind              string     `json:"kind"`
	Recipient         string     `json:"recipient"`
	Status            string     `json:"status"`
	ProviderMessageID *string    `json:"providerMessageId"`
	Attempts          int        `json:"attempts"`
	LastError         *string    `json:"lastError"`
	NextAttemptAt     *time.Time `json:"nextAttemptAt"`
	CreatedAt         time.Time  `json:"createdAt"`
	SentAt            *time.Time `json:"sentAt"`
	Payload           []byte     `json:"-"`
	Generation        int        `json:"-"`
}

type AdminUser struct {
	ID           string
	Email        string
	Name         string
	PasswordHash string
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

type AdminAction struct {
	ID         string         `json:"id"`
	AdminID    *string        `json:"adminId"`
	AdminEmail *string        `json:"adminEmail"`
	Action     string         `json:"action"`
	TargetType string         `json:"targetType"`
	TargetID   *string        `json:"targetId"`
	Reason     *string        `json:"reason"`
	Details    map[string]any `json:"details"`
	CreatedAt  time.Time      `json:"createdAt"`
	ResolvedAt *time.Time     `json:"resolvedAt"`
	Resolution *string        `json:"resolution"`
}
