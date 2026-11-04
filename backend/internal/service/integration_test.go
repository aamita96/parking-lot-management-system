//go:build integration

// Integration tests against a real Postgres started by Testcontainers (NFR-16).
// Run with: go test -tags integration ./internal/...
package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"plms/internal/apperr"
	"plms/internal/auth"
	"plms/internal/config"
	"plms/internal/domain"
	"plms/internal/email"
	"plms/internal/events"
	"plms/internal/notifier"
	"plms/internal/platform/db"
	"plms/internal/service"
	"plms/internal/store"
)

var dsn string

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("plms"), tcpostgres.WithUsername("plms"), tcpostgres.WithPassword("plms"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		panic(err)
	}
	dsn, err = pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	if err := db.Migrate(dsn); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = pg.Terminate(ctx)
	if code != 0 {
		panic("tests failed")
	}
}

type harness struct {
	svc    *service.Service
	pool   *pgxpool.Pool
	simIn  store.GateDevice
	simOut store.GateDevice
}

func setup(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Fresh data per test, keeping the seeded rate card, settings and simulator gates.
	if _, err := pool.Exec(ctx, `TRUNCATE email_notifications, outbox_events, idempotency_keys, gate_scans, tickets,
		email_codes, vehicles, owners, admin_actions CASCADE;
		UPDATE parking_spots SET status = 'AVAILABLE'; UPDATE lot_settings SET overflow_enabled = false;`); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Env: "test", SimulatorEnabled: true, DevExposeCodes: true, PublicURL: "http://localhost:3000",
		JWTSecret: []byte("0123456789abcdef0123456789abcdef"), CodeSecret: []byte("code-secret-0123"),
		OwnerTokenTTL: time.Hour, AdminAccessTTL: time.Minute, AdminRefreshTTL: time.Hour}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.New(pool, cfg, auth.NewTokens(cfg.JWTSecret), email.NewDevSender(), log)
	h := &harness{svc: svc, pool: pool}
	if h.simIn, err = svc.SimulatorGate(ctx, "SIM-IN"); err != nil {
		t.Fatal(err)
	}
	if h.simOut, err = svc.SimulatorGate(ctx, "SIM-OUT"); err != nil {
		t.Fatal(err)
	}
	return h
}

func code(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// register runs the self-registration flow and returns the owner session.
func (h *harness) register(t *testing.T, plate string, vt domain.VehicleType, mail string) service.OwnerSession {
	t.Helper()
	ctx := context.Background()
	res, err := h.svc.StartRegistration(ctx, service.VehicleInput{VehicleNumber: plate, VehicleType: string(vt), OwnerEmail: mail})
	if err != nil {
		t.Fatalf("register %s: %v", plate, err)
	}
	if res.DevCode == "" {
		t.Fatal("expected dev code in test mode")
	}
	sess, err := h.svc.VerifyRegistration(ctx, res.RegistrationID, res.DevCode)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	return sess
}

// Acceptance criteria 1, 3 and 4 (SRS 13.2).
func TestRegisterEnterPayExit(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	sess := h.register(t, "ka 01-ab 1234", domain.Car, "Owner@Example.com")
	if sess.Vehicle.VehicleNumber != "KA01AB1234" || sess.Vehicle.Status != domain.VehicleActive {
		t.Fatalf("vehicle = %+v", sess.Vehicle)
	}
	if sess.Owner.Email != "owner@example.com" {
		t.Errorf("email not lower-cased: %s", sess.Owner.Email)
	}

	before, _ := h.svc.Layout(ctx)
	in, err := h.svc.EntryScan(ctx, h.simIn, "KA01AB1234")
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if in.Spot.Code != "A3" || in.Owner.Email != "owner@example.com" || in.Result != "ADMITTED" {
		t.Fatalf("entry response = %+v", in)
	}
	after, _ := h.svc.Layout(ctx)
	if after.FreeCount != before.FreeCount-1 {
		t.Errorf("P board: %d -> %d", before.FreeCount, after.FreeCount)
	}

	stay := 200
	out, err := h.svc.ExitScan(ctx, h.simOut, service.ExitInput{Plate: "KA01AB1234", SimulatedStayMinutes: &stay})
	if err != nil {
		t.Fatalf("exit: %v", err)
	}
	if out.FeePaise != 16000 || out.BillableHours != 4 || out.Result != "EXIT_PENDING_PAYMENT" {
		t.Fatalf("exit response = %+v", out)
	}
	if out.Breakdown != "4 h × ₹40 = ₹160.00" {
		t.Errorf("breakdown = %q", out.Breakdown)
	}

	if _, err := h.svc.Pay(ctx, h.simOut, service.PaymentInput{TicketID: out.Ticket.ID, AmountPaise: 100, Method: domain.PaySimUPI}); code(err) != "AMOUNT_MISMATCH" {
		t.Errorf("wrong amount: got %v", err)
	}
	rc, err := h.svc.Pay(ctx, h.simOut, service.PaymentInput{TicketID: out.Ticket.ID, AmountPaise: 16000, Method: domain.PaySimUPI})
	if err != nil || rc.Status != "CLOSED" || !rc.OpenBarrier || rc.PaidPaise != 16000 {
		t.Fatalf("pay = %+v, %v", rc, err)
	}
	// FR-26: paying again never charges twice.
	_, err = h.svc.Pay(ctx, h.simOut, service.PaymentInput{TicketID: out.Ticket.ID, AmountPaise: 16000, Method: domain.PaySimUPI})
	if e, ok := apperr.As(err); !ok || e.Code != "TICKET_ALREADY_CLOSED" || e.Extra["receipt"] == nil {
		t.Errorf("second pay = %v", err)
	}
	final, _ := h.svc.Layout(ctx)
	if final.FreeCount != before.FreeCount {
		t.Errorf("spot not freed: %d vs %d", final.FreeCount, before.FreeCount)
	}

	evs, _ := h.svc.SimulatorEvents(ctx, 20)
	if len(evs) != 2 || evs[0].EventType != "VehicleExited" || evs[1].EventType != "VehicleEntered" {
		t.Fatalf("events = %+v", evs)
	}
	var env events.Envelope
	var payload []byte
	_ = h.pool.QueryRow(ctx, `SELECT payload FROM outbox_events WHERE event_type = 'VehicleExited'`).Scan(&payload)
	_ = json.Unmarshal(payload, &env)
	if !env.Simulated || env.Source != "SIMULATOR" || *env.Data.FeePaise != 16000 || env.Data.PaymentMethod != "SIMULATED_UPI" {
		t.Errorf("exit envelope = %+v", env)
	}
}

// Acceptance criterion 2: unverified vehicles wait.
func TestUnverifiedIsRejected(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	if _, err := h.svc.StartRegistration(ctx, service.VehicleInput{VehicleNumber: "MH12XY9999", VehicleType: "CAR", OwnerEmail: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
	_, err := h.svc.EntryScan(ctx, h.simIn, "MH12XY9999")
	if code(err) != "NOT_VERIFIED" {
		t.Fatalf("got %v", err)
	}
	var n int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM tickets`).Scan(&n)
	if n != 0 {
		t.Errorf("ticket created for unverified vehicle")
	}
	if _, err := h.svc.EntryScan(ctx, h.simIn, "DL01ZZ0001"); code(err) != "NOT_REGISTERED" {
		t.Errorf("unregistered: %v", err)
	}
	if _, err := h.svc.EntryScan(ctx, h.simIn, "???"); code(err) != "UNREADABLE_PLATE" {
		t.Errorf("unreadable: %v", err)
	}
	ex, _ := h.svc.Exceptions(ctx)
	if ex.Counts["rejectedScans"] != 3 {
		t.Errorf("exception queue = %d rejected scans", ex.Counts["rejectedScans"])
	}
}

// FR-5: someone else's number is refused and can be disputed.
func TestNumberOwnedByAnother(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	h.register(t, "KA01AB1234", domain.Car, "first@example.com")
	_, err := h.svc.StartRegistration(ctx, service.VehicleInput{VehicleNumber: "KA01AB1234", VehicleType: "CAR", OwnerEmail: "second@example.com"})
	if code(err) != "VEHICLE_ALREADY_REGISTERED" {
		t.Fatalf("got %v", err)
	}
	res, err := h.svc.StartRegistration(ctx, service.VehicleInput{VehicleNumber: "KA01AB1234", VehicleType: "CAR", OwnerEmail: "first@example.com"})
	if err != nil || res.Status != "ALREADY_REGISTERED" {
		t.Errorf("same owner: %+v %v", res, err)
	}
	if _, err := h.svc.OpenDispute(ctx, service.DisputeInput{VehicleNumber: "KA01AB1234", ReporterEmail: "second@example.com", Message: "This is my car"}); err != nil {
		t.Fatal(err)
	}
	ex, _ := h.svc.Exceptions(ctx)
	if ex.Counts["disputes"] != 1 {
		t.Errorf("disputes = %d", ex.Counts["disputes"])
	}
}

// FR-4: wrong codes are counted and the 6th attempt is refused even with the right code.
func TestCodeAttempts(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	res, err := h.svc.StartRegistration(ctx, service.VehicleInput{VehicleNumber: "KA02CD3456", VehicleType: "CAR", OwnerEmail: "c@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := h.svc.VerifyRegistration(ctx, res.RegistrationID, "000000x"); code(err) != "CODE_INVALID" {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := h.svc.VerifyRegistration(ctx, res.RegistrationID, res.DevCode); code(err) != "CODE_EXPIRED" {
		t.Errorf("after 5 wrong attempts: %v", err)
	}
}

// NFR-3: zero double allocations under 50 concurrent entry scans for the last free spot.
func TestConcurrentLastSpot(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	// Leave exactly one regular spot free.
	if _, err := h.pool.Exec(ctx, `UPDATE parking_spots SET status = 'OUT_OF_SERVICE' WHERE spot_type = 'REGULAR' AND code <> 'A3'`); err != nil {
		t.Fatal(err)
	}
	defer h.pool.Exec(ctx, `UPDATE parking_spots SET status = 'AVAILABLE'`)
	plates := make([]string, 50)
	for i := range plates {
		plates[i] = "KA01AA" + pad4(i)
		owner, _ := store.UpsertOwner(ctx, h.pool, "racer"+pad4(i)+"@example.com", nil)
		_ = store.MarkOwnerVerified(ctx, h.pool, owner.ID)
		if _, err := store.InsertVehicle(ctx, h.pool, plates[i], domain.Car, owner.ID, nil, domain.VehicleActive); err != nil {
			t.Fatal(err)
		}
	}
	var admitted, noSpot int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, p := range plates {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			<-start
			_, err := h.svc.EntryScan(ctx, h.simIn, p)
			switch code(err) {
			case "":
				atomic.AddInt32(&admitted, 1)
			case "NO_SPOT_AVAILABLE":
				atomic.AddInt32(&noSpot, 1)
			default:
				t.Errorf("unexpected: %v", err)
			}
		}(p)
	}
	close(start)
	wg.Wait()
	if admitted != 1 || noSpot != 49 {
		t.Fatalf("admitted=%d noSpot=%d", admitted, noSpot)
	}
	var open int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE status = 'ACTIVE'`).Scan(&open)
	if open != 1 {
		t.Errorf("open tickets = %d", open)
	}
}

func pad4(i int) string {
	s := "0000" + itoa(i)
	return s[len(s)-4:]
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// FR-19, FR-24, FR-25, BR-3 and the overflow rule.
func TestGateEdgeCases(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	h.register(t, "KA05MN4321", domain.Motorcycle, "m@example.com")

	first, err := h.svc.EntryScan(ctx, h.simIn, "KA05MN4321")
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.svc.EntryScan(ctx, h.simIn, "ka05 mn-4321")
	if err != nil || again.Ticket.ID != first.Ticket.ID || !again.Deduplicated {
		t.Fatalf("repeat read = %+v %v", again, err)
	}

	zero := 5
	out, err := h.svc.ExitScan(ctx, h.simOut, service.ExitInput{Plate: "KA05MN4321", SimulatedStayMinutes: &zero})
	if err != nil || out.Result != "EXIT_CLOSED" || out.FeePaise != 0 || !out.OpenBarrier {
		t.Fatalf("grace exit = %+v %v", out, err)
	}
	if _, err := h.svc.ExitScan(ctx, h.simOut, service.ExitInput{Plate: "MH12XY0001"}); code(err) != "NO_ACTIVE_TICKET" {
		t.Errorf("no ticket: %v", err)
	}

	// Fill the three compact spots (A1, A2, B1), then a motorcycle only fits with overflow on.
	for i, p := range []string{"KA05MN0001", "KA05MN0002", "KA05MN0004", "KA05MN0003"} {
		h.register(t, p, domain.Motorcycle, "m"+itoa(i)+"@example.com")
	}
	for _, p := range []string{"KA05MN0001", "KA05MN0002", "KA05MN0004"} {
		if _, err := h.svc.EntryScan(ctx, h.simIn, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.svc.EntryScan(ctx, h.simIn, "KA05MN0003"); code(err) != "NO_SPOT_AVAILABLE" {
		t.Fatalf("expected full: %v", err)
	}
	av, _ := h.svc.Availability(ctx)
	if !av.Panels[0].Full {
		t.Error("motorcycle panel should be Full")
	}
	_, _ = h.pool.Exec(ctx, `UPDATE lot_settings SET overflow_enabled = true`)
	// A second gate is outside the first gate's 30 s repeat-read window (FR-19 is per gate).
	_, keyHash := auth.NewGateKey()
	in2, err := store.InsertGate(ctx, h.pool, "IN-T"+itoa(int(time.Now().UnixNano()%100000)), "ENTRY", keyHash)
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.svc.EntryScan(ctx, in2, "KA05MN0003")
	if err != nil || r.Spot.SpotType != domain.Regular {
		t.Fatalf("overflow entry = %+v %v", r, err)
	}
}

// Acceptance criterion 6: a redelivered event produces exactly one email.
func TestNotifierIdempotent(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	h.register(t, "KA01AB1234", domain.Car, "o@example.com")
	in, err := h.svc.EntryScan(ctx, h.simIn, "KA01AB1234")
	if err != nil {
		t.Fatal(err)
	}
	var eventID string
	var payload []byte
	if err := h.pool.QueryRow(ctx, `SELECT event_id, payload FROM outbox_events WHERE aggregate_id = $1`, in.Ticket.ID).Scan(&eventID, &payload); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.InsertNotification(ctx, h.pool, eventID, in.Ticket.ID, "ENTRY", "o@example.com", payload); err != nil {
			t.Fatal(err)
		}
	}
	sender := &countingSender{}
	d := &notifier.Dispatcher{Pool: h.pool, Sender: sender, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	nudge := make(chan struct{})
	d.Nudge = nudge
	go d.Run(cctx)
	<-cctx.Done()
	if sender.n.Load() != 1 {
		t.Fatalf("emails sent = %d, want 1", sender.n.Load())
	}
	var status string
	_ = h.pool.QueryRow(ctx, `SELECT status FROM email_notifications WHERE event_id = $1`, eventID).Scan(&status)
	if status != "SENT" {
		t.Errorf("status = %s", status)
	}
}

type countingSender struct{ n atomic.Int32 }

func (c *countingSender) Send(ctx context.Context, m email.Message) (string, error) {
	if m.To == "" || m.Subject == "" {
		return "", errors.New("empty message")
	}
	c.n.Add(1)
	return "test-id", nil
}
