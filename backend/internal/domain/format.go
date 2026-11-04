package domain

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // IST must work in minimal containers and on Windows
)

// IST is Asia/Kolkata; all times are stored in UTC and shown in IST (SRS 1.4).
var IST = mustLoad("Asia/Kolkata")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("IST", 5*3600+1800)
	}
	return loc
}

// Rupees formats paise as ₹1,300.00 using Indian digit grouping.
func Rupees(paise int64) string {
	neg := paise < 0
	if neg {
		paise = -paise
	}
	whole := paise / 100
	frac := paise % 100
	s := groupIndian(fmt.Sprintf("%d", whole))
	out := fmt.Sprintf("₹%s.%02d", s, frac)
	if neg {
		out = "-" + out
	}
	return out
}

// RupeesShort drops ".00" for whole amounts: ₹40, ₹1,000, ₹12.50.
func RupeesShort(paise int64) string {
	if paise%100 == 0 {
		return strings.TrimSuffix(Rupees(paise), ".00")
	}
	return Rupees(paise)
}

// groupIndian inserts commas as 12,34,567 (last 3 digits, then pairs).
func groupIndian(d string) string {
	if len(d) <= 3 {
		return d
	}
	head, tail := d[:len(d)-3], d[len(d)-3:]
	var parts []string
	for len(head) > 2 {
		parts = append([]string{head[len(head)-2:]}, parts...)
		head = head[:len(head)-2]
	}
	if head != "" {
		parts = append([]string{head}, parts...)
	}
	return strings.Join(parts, ",") + "," + tail
}

// DateTimeIST renders "04 Nov 2026, 10:05 AM IST".
func DateTimeIST(t time.Time) string {
	return t.In(IST).Format("02 Jan 2006, 03:04 PM") + " IST"
}

// ClockIST renders "10:05 AM".
func ClockIST(t time.Time) string {
	return t.In(IST).Format("03:04 PM")
}

// DateIST renders "04 Nov 2026".
func DateIST(t time.Time) string {
	return t.In(IST).Format("02 Jan 2006")
}

// HumanDuration renders minutes as "3 h 20 min", "45 min", "1 d 2 h 10 min".
func HumanDuration(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	d := minutes / minutesPerDay
	h := (minutes % minutesPerDay) / 60
	m := minutes % 60
	var parts []string
	if d > 0 {
		parts = append(parts, fmt.Sprintf("%d d", d))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%d h", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%d min", m))
	}
	return strings.Join(parts, " ")
}

// FeeBreakdown renders the receipt line, e.g. "4 h × ₹40 = ₹160.00" or
// "1 day × ₹1,000 + 3 h × ₹100 = ₹1,300.00".
func FeeBreakdown(f Fee, r Rate) string {
	if f.WithinGrace {
		return fmt.Sprintf("No charge, within %d free minutes", r.GraceMinutes)
	}
	var parts []string
	if f.FullDays > 0 {
		unit := "day"
		if f.FullDays > 1 {
			unit = "days"
		}
		parts = append(parts, fmt.Sprintf("%d %s × %s", f.FullDays, unit, RupeesShort(r.DailyCapPaise)))
	}
	if f.RemainderHours > 0 {
		p := fmt.Sprintf("%d h × %s", f.RemainderHours, RupeesShort(r.HourlyRatePaise))
		if f.RemainderCapped {
			p += fmt.Sprintf(" (capped at %s)", RupeesShort(r.DailyCapPaise))
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " + ") + " = " + Rupees(f.FeePaise)
}

// RateSummary renders "₹40/hour after 10 free minutes, max ₹300/day".
func RateSummary(r Rate) string {
	return fmt.Sprintf("%s/hour after %d free minutes, max %s/day",
		RupeesShort(r.HourlyRatePaise), r.GraceMinutes, RupeesShort(r.DailyCapPaise))
}

// MaskEmail turns owner@example.com into o***@example.com (FR-20, NFR-10).
func MaskEmail(e string) string {
	at := strings.LastIndexByte(e, '@')
	if at <= 0 {
		return "***"
	}
	return e[:1] + "***" + e[at:]
}

// MaskMobile keeps the last 4 digits: +91******3210.
func MaskMobile(m string) string {
	if len(m) < 8 {
		return "****"
	}
	return m[:3] + strings.Repeat("*", len(m)-7) + m[len(m)-4:]
}
