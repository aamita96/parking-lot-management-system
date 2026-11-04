package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/domain"
	"plms/internal/platform/db"
)

const ticketSelect = `
	SELECT t.id, t.ticket_number, t.vehicle_id, t.spot_id, t.pricing_rule_id, t.entry_gate_id, t.exit_gate_id,
	       t.vehicle_type, t.spot_type, t.owner_email, t.entry_time, t.exit_time, t.duration_minutes, t.billable_hours,
	       t.fee_paise, t.paid_paise, t.payment_method, t.paid_at, t.status, t.is_simulated, t.source,
	       v.vehicle_number, v.owner_id, s.code, s.floor, s.row_label, s.position,
	       p.hourly_rate_paise, p.daily_cap_paise, p.grace_minutes
	FROM tickets t
	JOIN vehicles v ON v.id = t.vehicle_id
	JOIN parking_spots s ON s.id = t.spot_id
	JOIN pricing_rules p ON p.id = t.pricing_rule_id`

func scanTicket(row pgx.Row) (Ticket, error) {
	var t Ticket
	err := row.Scan(&t.ID, &t.TicketNumber, &t.VehicleID, &t.SpotID, &t.PricingRuleID, &t.EntryGateID, &t.ExitGateID,
		&t.VehicleType, &t.SpotType, &t.OwnerEmail, &t.EntryTime, &t.ExitTime, &t.DurationMinutes, &t.BillableHours,
		&t.FeePaise, &t.PaidPaise, &t.PaymentMethod, &t.PaidAt, &t.Status, &t.IsSimulated, &t.Source,
		&t.VehicleNumber, &t.OwnerID, &t.SpotCode, &t.SpotFloor, &t.SpotRow, &t.SpotPosition,
		&t.Rate.HourlyRatePaise, &t.Rate.DailyCapPaise, &t.Rate.GraceMinutes)
	return t, err
}

func collectTickets(rows pgx.Rows, err error) ([]Ticket, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func TicketByID(ctx context.Context, q db.DBTX, id string) (Ticket, error) {
	return scanTicket(q.QueryRow(ctx, ticketSelect+` WHERE t.id = $1`, id))
}

// TicketByIDForUpdate locks the ticket row (payment, manual exit).
func TicketByIDForUpdate(ctx context.Context, q db.DBTX, id string) (Ticket, error) {
	return scanTicket(q.QueryRow(ctx, ticketSelect+` WHERE t.id = $1 FOR UPDATE OF t`, id))
}

// OpenTicketForVehicle returns the vehicle's ACTIVE or PAYMENT_PENDING ticket, locked (exit scan).
func OpenTicketForVehicle(ctx context.Context, q db.DBTX, vehicleID string, lock bool) (Ticket, error) {
	sql := ticketSelect + ` WHERE t.vehicle_id = $1 AND t.status IN ('ACTIVE', 'PAYMENT_PENDING')`
	if lock {
		sql += ` FOR UPDATE OF t`
	}
	return scanTicket(q.QueryRow(ctx, sql, vehicleID))
}

// NextTicketNumber formats PLM-YYYYMMDD-000123 using the IST date and a global sequence.
func NextTicketNumber(ctx context.Context, q db.DBTX, now time.Time) (string, error) {
	var n int64
	if err := q.QueryRow(ctx, `SELECT nextval('ticket_number_seq')`).Scan(&n); err != nil {
		return "", err
	}
	return fmt.Sprintf("PLM-%s-%06d", now.In(domain.IST).Format("20060102"), n), nil
}

type NewTicket struct {
	TicketNumber  string
	VehicleID     string
	SpotID        string
	PricingRuleID string
	EntryGateID   *string
	VehicleType   domain.VehicleType
	SpotType      domain.SpotType
	OwnerEmail    string
	IsSimulated   bool
	Source        string
}

// InsertTicket creates an ACTIVE ticket with server-side entry time (FR-15).
func InsertTicket(ctx context.Context, q db.DBTX, n NewTicket) (string, error) {
	var id string
	err := q.QueryRow(ctx, `INSERT INTO tickets
		(ticket_number, vehicle_id, spot_id, pricing_rule_id, entry_gate_id, vehicle_type, spot_type, owner_email, is_simulated, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		n.TicketNumber, n.VehicleID, n.SpotID, n.PricingRuleID, n.EntryGateID, string(n.VehicleType), string(n.SpotType),
		n.OwnerEmail, n.IsSimulated, n.Source).Scan(&id)
	return id, err
}

// LockFee records exit time and the calculated fee (FR-21).
func LockFee(ctx context.Context, q db.DBTX, id string, exit time.Time, exitGate *string, f domain.Fee, status string) error {
	_, err := q.Exec(ctx, `UPDATE tickets SET exit_time = $2, exit_gate_id = COALESCE($3, exit_gate_id), duration_minutes = $4,
		billable_hours = $5, fee_paise = $6, status = $7 WHERE id = $1`,
		id, exit, exitGate, f.DurationMinutes, f.BillableHours, f.FeePaise, status)
	return err
}

// CloseTicket marks the ticket paid and closed (FR-23).
func CloseTicket(ctx context.Context, q db.DBTX, id string, paid int64, method string, status string) error {
	_, err := q.Exec(ctx, `UPDATE tickets SET paid_paise = $2, payment_method = $3, paid_at = now(), status = $4 WHERE id = $1`,
		id, paid, method, status)
	return err
}

type TicketFilter struct {
	Query     string
	Status    string // ACTIVE | PAYMENT_PENDING | CLOSED | CANCELLED | OPEN
	Simulated *bool
	VehicleID string
	OwnerID   string
	Limit     int
	Offset    int
}

func SearchTickets(ctx context.Context, q db.DBTX, f TicketFilter) ([]Ticket, int, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Query != "" {
		add(`(t.ticket_number ILIKE '%%' || $%[1]d || '%%' OR v.vehicle_number ILIKE '%%' || $%[1]d || '%%' OR t.owner_email ILIKE '%%' || $%[1]d || '%%' OR s.code ILIKE $%[1]d)`, f.Query)
	}
	switch f.Status {
	case "":
	case "OPEN":
		where = append(where, `t.status IN ('ACTIVE', 'PAYMENT_PENDING')`)
	default:
		add(`t.status = $%d::ticket_status`, f.Status)
	}
	if f.Simulated != nil {
		add(`t.is_simulated = $%d`, *f.Simulated)
	}
	if f.VehicleID != "" {
		add(`t.vehicle_id = $%d`, f.VehicleID)
	}
	if f.OwnerID != "" {
		add(`v.owner_id = $%d`, f.OwnerID)
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM tickets t JOIN vehicles v ON v.id = t.vehicle_id
		JOIN parking_spots s ON s.id = t.spot_id WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, clampLimit(f.Limit), f.Offset)
	list, err := collectTickets(q.Query(ctx, ticketSelect+` WHERE `+cond+
		fmt.Sprintf(` ORDER BY t.entry_time DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...))
	return list, total, err
}

// StalePaymentPending lists tickets stuck in PAYMENT_PENDING longer than d (FR-27).
func StalePaymentPending(ctx context.Context, q db.DBTX, d time.Duration) ([]Ticket, error) {
	return collectTickets(q.Query(ctx, ticketSelect+` WHERE t.status = 'PAYMENT_PENDING'
		AND t.exit_time < now() - make_interval(secs => $1) ORDER BY t.exit_time`, d.Seconds()))
}

// OpenSimulatedTickets lists simulator tickets still open (FR-48 reset).
func OpenSimulatedTickets(ctx context.Context, q db.DBTX) ([]Ticket, error) {
	return collectTickets(q.Query(ctx, ticketSelect+` WHERE t.is_simulated AND t.status IN ('ACTIVE', 'PAYMENT_PENDING') FOR UPDATE OF t`))
}

func CancelTicket(ctx context.Context, q db.DBTX, id string) error {
	_, err := q.Exec(ctx, `UPDATE tickets SET status = 'CANCELLED', exit_time = COALESCE(exit_time, GREATEST(now(), entry_time)) WHERE id = $1`, id)
	return err
}

// OpenTicketsBySpot maps spot ID to its open ticket (simulator layout).
func OpenTicketsBySpot(ctx context.Context, q db.DBTX) (map[string]Ticket, error) {
	list, err := collectTickets(q.Query(ctx, ticketSelect+` WHERE t.status IN ('ACTIVE', 'PAYMENT_PENDING')`))
	if err != nil {
		return nil, err
	}
	out := make(map[string]Ticket, len(list))
	for _, t := range list {
		out[t.SpotID] = t
	}
	return out, nil
}
