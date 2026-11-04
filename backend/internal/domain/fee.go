package domain

import "time"

// Rate is one rate card entry, in paise (BR-12).
type Rate struct {
	HourlyRatePaise int64 `json:"hourlyRatePaise"`
	DailyCapPaise   int64 `json:"dailyCapPaise"`
	GraceMinutes    int   `json:"graceMinutes"`
}

// Fee is the result of the fee calculation, with the parts needed for a receipt.
type Fee struct {
	DurationMinutes int   `json:"durationMinutes"`
	BillableHours   int   `json:"billableHours"`
	WithinGrace     bool  `json:"withinGrace"`
	FullDays        int   `json:"fullDays"`
	RemainderHours  int   `json:"remainderHours"`
	RemainderPaise  int64 `json:"remainderPaise"` // after the daily cap
	RemainderCapped bool  `json:"remainderCapped"`
	FeePaise        int64 `json:"feePaise"`
}

const minutesPerDay = 24 * 60

// DurationMinutes is exit − entry in whole minutes (BR-7). Partial minutes are dropped.
func DurationMinutes(entry, exit time.Time) int {
	d := exit.Sub(entry)
	if d < 0 {
		return 0
	}
	return int(d / time.Minute)
}

// CalculateFee applies BR-6 to BR-10 for a stay of d minutes:
//
//	fee = 0                                              if d <= grace
//	fee = floor(d/1440)*cap + min(cap, ceil((d mod 1440)/60)*rate)   otherwise
//
// Billable hours are the stay rounded up to whole hours, minimum 1.
func CalculateFee(d int, r Rate) Fee {
	if d < 0 {
		d = 0
	}
	f := Fee{DurationMinutes: d}
	if d <= r.GraceMinutes {
		f.WithinGrace = true
		return f
	}
	// d > grace >= 0, so d >= 1 and ceil(d/60) is already at least 1 (minimum 1 hour).
	f.BillableHours = ceilDiv(d, 60)
	f.FullDays = d / minutesPerDay
	f.RemainderHours = ceilDiv(d%minutesPerDay, 60)
	remainder := int64(f.RemainderHours) * r.HourlyRatePaise
	if remainder > r.DailyCapPaise {
		remainder = r.DailyCapPaise
		f.RemainderCapped = true
	}
	f.RemainderPaise = remainder
	f.FeePaise = int64(f.FullDays)*r.DailyCapPaise + remainder
	return f
}

func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}
