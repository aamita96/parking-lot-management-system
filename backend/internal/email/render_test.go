package email

import (
	"strings"
	"testing"
	"time"

	"plms/internal/domain"
	"plms/internal/events"
)

func sampleEnvelope() events.Envelope {
	entry := time.Date(2026, 11, 4, 4, 35, 0, 0, time.UTC) // 10:05 IST
	return events.Envelope{
		EventID:   "e1",
		EventType: domain.EventEntered,
		Data: events.TicketData{
			TicketNumber: "PLM-20261104-000123",
			Vehicle:      events.VehicleRef{VehicleNumber: "KA01AB1234", VehicleType: domain.Car, OwnerEmail: "owner@example.com"},
			Spot:         events.SpotRef{Code: "R-1-014", SpotType: domain.Regular, Floor: 1},
			EntryTime:    entry,
			Rate:         domain.Rate{HourlyRatePaise: 4000, DailyCapPaise: 30000, GraceMinutes: 10},
		},
	}
}

func TestEntryEmail(t *testing.T) {
	m, err := EntryEmail(sampleEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	if m.Subject != "Checked in: KA01AB1234 at spot R-1-014 — Ticket PLM-20261104-000123" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"04 Nov 2026, 10:05 AM IST", "₹40/hour after 10 free minutes, max ₹300/day", "R-1-014 · Regular · Floor 1"} {
		if !strings.Contains(m.HTML, want) || !strings.Contains(m.Text, want) {
			t.Errorf("entry email missing %q", want)
		}
	}
}

func TestExitEmail(t *testing.T) {
	e := sampleEnvelope()
	e.EventType = domain.EventExited
	exit := e.Data.EntryTime.Add(200 * time.Minute)
	dur, hours, fee, paid := 200, 4, int64(16000), int64(16000)
	e.Data.ExitTime, e.Data.DurationMinutes, e.Data.BillableHours = &exit, &dur, &hours
	e.Data.FeePaise, e.Data.PaidPaise, e.Data.PaymentMethod, e.Data.PaidAt = &fee, &paid, domain.PaySimUPI, &exit
	m, err := ExitEmail(e)
	if err != nil {
		t.Fatal(err)
	}
	if m.Subject != "Checked out: KA01AB1234 — ₹160.00 for 3 h 20 min" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"10:05 AM → 01:25 PM IST, 04 Nov 2026", "3 h 20 min (billed as 4 h)", "4 h × ₹40 = ₹160.00", "Paid ₹160.00 by UPI (simulated) at 01:25 PM IST"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("exit email missing %q\n%s", want, m.Text)
		}
	}
}

func TestCodeEmail(t *testing.T) {
	m, err := CodeEmail("o@example.com", PurposeVerify, "483920", 10, "KA01AB1234", domain.Car)
	if err != nil {
		t.Fatal(err)
	}
	if m.Subject != "Your PLMS code: 483920" || !strings.Contains(m.HTML, "KA01AB1234 · Car") ||
		!strings.Contains(m.Text, "If you didn't try to register this vehicle, ignore this email.") {
		t.Errorf("unexpected code email: %s\n%s", m.Subject, m.Text)
	}
}
