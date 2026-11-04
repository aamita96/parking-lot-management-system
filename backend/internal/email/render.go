package email

import (
	"bytes"
	"embed"
	"fmt"
	htmltpl "html/template"
	texttpl "text/template"

	"plms/internal/domain"
	"plms/internal/events"
)

//go:embed templates/*
var templateFS embed.FS

type pair struct{ Label, Value string }

var funcs = map[string]any{"row": func(l, v string) pair { return pair{l, v} }}

var (
	htmlTpls = map[string]*htmltpl.Template{}
	textTpls = map[string]*texttpl.Template{}
)

func init() {
	for _, name := range []string{"code", "entry", "exit"} {
		htmlTpls[name] = htmltpl.Must(htmltpl.New("layout").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html"))
		textTpls[name] = texttpl.Must(texttpl.New(name + ".txt").ParseFS(templateFS, "templates/"+name+".txt"))
	}
}

func render(name, subject, to string, data any) (Message, error) {
	var h, t bytes.Buffer
	if err := htmlTpls[name].ExecuteTemplate(&h, "layout", data); err != nil {
		return Message{}, err
	}
	if err := textTpls[name].Execute(&t, data); err != nil {
		return Message{}, err
	}
	return Message{To: to, Subject: subject, HTML: h.String(), Text: t.String()}, nil
}

// Code purposes.
const (
	PurposeVerify      = "VERIFY"
	PurposeSignIn      = "SIGN_IN"
	PurposeChangeEmail = "CHG_EMAIL"
)

type codeData struct {
	Subject, Heading, Intro, Code, VehicleNumber, VehicleLine, SafetyLine string
	ValidMinutes                                                          int
	Simulated                                                             bool
}

// CodeEmail renders the verification / sign-in code email (SRS 9.4).
func CodeEmail(to, purpose, code string, validMinutes int, vehicleNumber string, vt domain.VehicleType) (Message, error) {
	d := codeData{
		Subject:      "Your PLMS code: " + code,
		Code:         code,
		ValidMinutes: validMinutes,
	}
	switch purpose {
	case PurposeSignIn:
		d.Heading = "Sign in to My vehicles"
		d.Intro = "Enter this code to see your vehicles and parking tickets."
		d.SafetyLine = "If you didn't try to sign in, ignore this email. Your account stays safe."
	case PurposeChangeEmail:
		d.Heading = "Confirm your new email"
		d.Intro = "Enter this code to make this address the contact email for your vehicles."
		d.SafetyLine = "If you didn't ask to change your email, ignore this email."
	default:
		d.Heading = "Confirm your email"
		d.Intro = "Enter this code to finish registering your vehicle. It can park as soon as you do."
		d.SafetyLine = "If you didn't try to register this vehicle, ignore this email."
		if vehicleNumber != "" {
			d.VehicleNumber = vehicleNumber
			d.VehicleLine = vehicleNumber + " · " + vt.Label()
		}
	}
	return render("code", d.Subject, to, d)
}

type ticketData struct {
	Subject, TicketNumber, VehicleNumber, VehicleLine, SpotCode, SpotLine string
	EntryTime, Rate                                                       string
	EntryExit, DurationLine, Breakdown, Total, PaidLine                   string
	Simulated                                                             bool
}

func baseTicket(e events.Envelope) ticketData {
	d := e.Data
	return ticketData{
		TicketNumber:  d.TicketNumber,
		VehicleNumber: d.Vehicle.VehicleNumber,
		VehicleLine:   d.Vehicle.VehicleNumber + " · " + d.Vehicle.VehicleType.Label(),
		SpotCode:      d.Spot.Code,
		SpotLine:      fmt.Sprintf("%s · %s · Floor %d", d.Spot.Code, d.Spot.SpotType.Label(), d.Spot.Floor),
		EntryTime:     domain.DateTimeIST(d.EntryTime),
		Rate:          domain.RateSummary(d.Rate),
		Simulated:     e.Simulated,
	}
}

// EntryEmail renders the check-in email (SRS 9.2).
func EntryEmail(e events.Envelope) (Message, error) {
	t := baseTicket(e)
	t.Subject = fmt.Sprintf("Checked in: %s at spot %s — Ticket %s", t.VehicleNumber, t.SpotCode, t.TicketNumber)
	return render("entry", t.Subject, e.Data.Vehicle.OwnerEmail, t)
}

// ExitEmail renders the receipt sent after payment (SRS 9.3).
func ExitEmail(e events.Envelope) (Message, error) {
	d := e.Data
	if d.ExitTime == nil || d.DurationMinutes == nil || d.FeePaise == nil {
		return Message{}, fmt.Errorf("exit event %s is missing exit fields", e.EventID)
	}
	t := baseTicket(e)
	fee := domain.CalculateFee(*d.DurationMinutes, d.Rate)
	fee.FeePaise = *d.FeePaise // the locked fee is authoritative
	t.Total = domain.Rupees(*d.FeePaise)
	t.Breakdown = domain.FeeBreakdown(fee, d.Rate)
	if d.PaymentMethod == domain.PayWaived {
		t.Breakdown = "Waived by the car park office"
	}
	t.DurationLine = domain.HumanDuration(*d.DurationMinutes)
	if fee.WithinGrace {
		t.DurationLine += " (free)"
	} else {
		t.DurationLine += fmt.Sprintf(" (billed as %d h)", fee.BillableHours)
	}
	t.EntryExit = fmt.Sprintf("%s → %s IST, %s", domain.ClockIST(d.EntryTime), domain.ClockIST(*d.ExitTime), domain.DateIST(d.EntryTime))
	if domain.DateIST(d.EntryTime) != domain.DateIST(*d.ExitTime) {
		t.EntryExit = fmt.Sprintf("%s → %s", domain.DateTimeIST(d.EntryTime), domain.DateTimeIST(*d.ExitTime))
	}
	paid := int64(0)
	if d.PaidPaise != nil {
		paid = *d.PaidPaise
	}
	switch {
	case d.PaymentMethod == domain.PayNoCharge || (*d.FeePaise == 0 && d.PaymentMethod == ""):
		t.PaidLine = "No charge — you left within the free period."
	case d.PaymentMethod == domain.PayWaived:
		t.PaidLine = "No payment needed — the fee was waived."
	default:
		at := ""
		if d.PaidAt != nil {
			at = " at " + domain.ClockIST(*d.PaidAt) + " IST"
		}
		t.PaidLine = fmt.Sprintf("Paid %s by %s%s", domain.Rupees(paid), domain.PaymentMethodLabel(d.PaymentMethod), at)
	}
	t.Subject = fmt.Sprintf("Checked out: %s — %s for %s", t.VehicleNumber, t.Total, domain.HumanDuration(*d.DurationMinutes))
	return render("exit", t.Subject, d.Vehicle.OwnerEmail, t)
}
