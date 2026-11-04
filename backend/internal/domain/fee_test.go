package domain

import (
	"testing"
	"time"
)

var (
	carRate   = Rate{HourlyRatePaise: 4000, DailyCapPaise: 30000, GraceMinutes: 10}
	motoRate  = Rate{HourlyRatePaise: 2000, DailyCapPaise: 15000, GraceMinutes: 10}
	truckRate = Rate{HourlyRatePaise: 10000, DailyCapPaise: 100000, GraceMinutes: 10}
)

// Worked examples from SRS 5.4 plus every branch of CalculateFee (NFR-15).
func TestCalculateFee(t *testing.T) {
	cases := []struct {
		name      string
		minutes   int
		rate      Rate
		wantFee   int64
		wantHours int
		grace     bool
		capped    bool
		days      int
	}{
		{"car within grace 7 min", 7, carRate, 0, 0, true, false, 0},
		{"exactly grace is free", 10, carRate, 0, 0, true, false, 0},
		{"negative duration treated as zero", -5, carRate, 0, 0, true, false, 0},
		{"one minute past grace bills 1 h", 11, carRate, 4000, 1, false, false, 0},
		{"car 200 min = 4 h x 40", 200, carRate, 16000, 4, false, false, 0},
		{"motorcycle 50 min in oversized pays moto rate", 50, motoRate, 2000, 1, false, false, 0},
		{"car 630 min capped at 300", 630, carRate, 30000, 11, false, true, 0},
		{"truck 26 h 10 min", 1570, truckRate, 130000, 27, false, false, 1},
		{"exactly 24 h: remainder term is 0", 1440, carRate, 30000, 24, false, false, 1},
		{"48 h 1 min", 2881, carRate, 64000, 49, false, false, 2},
		{"zero grace, 1 minute", 1, Rate{HourlyRatePaise: 4000, DailyCapPaise: 30000}, 4000, 1, false, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := CalculateFee(c.minutes, c.rate)
			if f.FeePaise != c.wantFee {
				t.Errorf("fee = %d, want %d", f.FeePaise, c.wantFee)
			}
			if f.BillableHours != c.wantHours {
				t.Errorf("billable hours = %d, want %d", f.BillableHours, c.wantHours)
			}
			if f.WithinGrace != c.grace {
				t.Errorf("withinGrace = %v, want %v", f.WithinGrace, c.grace)
			}
			if f.RemainderCapped != c.capped {
				t.Errorf("capped = %v, want %v", f.RemainderCapped, c.capped)
			}
			if f.FullDays != c.days {
				t.Errorf("days = %d, want %d", f.FullDays, c.days)
			}
		})
	}
}

func TestDurationMinutes(t *testing.T) {
	entry := time.Date(2026, 11, 4, 4, 35, 0, 0, time.UTC)
	if got := DurationMinutes(entry, entry.Add(200*time.Minute+59*time.Second)); got != 200 {
		t.Errorf("got %d, want 200 (partial minutes dropped)", got)
	}
	if got := DurationMinutes(entry, entry.Add(-time.Minute)); got != 0 {
		t.Errorf("got %d, want 0 for exit before entry", got)
	}
}

func TestFeeBreakdown(t *testing.T) {
	cases := []struct {
		minutes int
		rate    Rate
		want    string
	}{
		{200, carRate, "4 h × ₹40 = ₹160.00"},
		{1570, truckRate, "1 day × ₹1,000 + 3 h × ₹100 = ₹1,300.00"},
		{630, carRate, "11 h × ₹40 (capped at ₹300) = ₹300.00"},
		{2880, carRate, "2 days × ₹300 = ₹600.00"},
		{5, carRate, "No charge, within 10 free minutes"},
	}
	for _, c := range cases {
		if got := FeeBreakdown(CalculateFee(c.minutes, c.rate), c.rate); got != c.want {
			t.Errorf("FeeBreakdown(%d) = %q, want %q", c.minutes, got, c.want)
		}
	}
}

func TestRupees(t *testing.T) {
	cases := map[int64]string{
		0:         "₹0.00",
		16000:     "₹160.00",
		130000:    "₹1,300.00",
		12345678:  "₹1,23,456.78",
		-5050:     "-₹50.50",
		999:       "₹9.99",
		100000000: "₹10,00,000.00",
	}
	for in, want := range cases {
		if got := Rupees(in); got != want {
			t.Errorf("Rupees(%d) = %q, want %q", in, got, want)
		}
	}
	if got := RupeesShort(4000); got != "₹40" {
		t.Errorf("RupeesShort = %q", got)
	}
	if got := RupeesShort(4050); got != "₹40.50" {
		t.Errorf("RupeesShort = %q", got)
	}
}

func TestHumanDurationAndTimes(t *testing.T) {
	cases := map[int]string{45: "45 min", 60: "1 h", 200: "3 h 20 min", 1570: "1 d 2 h 10 min", 1440: "1 d"}
	for in, want := range cases {
		if got := HumanDuration(in); got != want {
			t.Errorf("HumanDuration(%d) = %q, want %q", in, got, want)
		}
	}
	entry := time.Date(2026, 11, 4, 4, 35, 0, 0, time.UTC)
	if got := DateTimeIST(entry); got != "04 Nov 2026, 10:05 AM IST" {
		t.Errorf("DateTimeIST = %q", got)
	}
	if got := RateSummary(carRate); got != "₹40/hour after 10 free minutes, max ₹300/day" {
		t.Errorf("RateSummary = %q", got)
	}
}

func TestAllocationOrder(t *testing.T) {
	if got := AllowedSpots(Motorcycle, false); len(got) != 1 || got[0] != Compact {
		t.Errorf("moto no overflow = %v", got)
	}
	if got := AllowedSpots(Motorcycle, true); len(got) != 3 || got[1] != Regular || got[2] != Oversized {
		t.Errorf("moto overflow = %v", got)
	}
	if got := AllowedSpots(Car, true); len(got) != 2 || got[0] != Regular || got[1] != Oversized {
		t.Errorf("car overflow = %v", got)
	}
	if got := AllowedSpots(Truck, true); len(got) != 1 || got[0] != Oversized {
		t.Errorf("truck overflow = %v", got)
	}
}
