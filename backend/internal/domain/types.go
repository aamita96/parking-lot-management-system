// Package domain holds PLMS enums and pure business rules (plates, fees, formatting).
package domain

type VehicleType string

const (
	Motorcycle VehicleType = "MOTORCYCLE"
	Car        VehicleType = "CAR"
	Truck      VehicleType = "TRUCK"
)

var VehicleTypes = []VehicleType{Motorcycle, Car, Truck}

func (v VehicleType) Valid() bool {
	return v == Motorcycle || v == Car || v == Truck
}

// Label is the human name used in emails and UI ("Car").
func (v VehicleType) Label() string {
	switch v {
	case Motorcycle:
		return "Motorcycle"
	case Car:
		return "Car"
	case Truck:
		return "Truck"
	}
	return string(v)
}

type SpotType string

const (
	Compact   SpotType = "COMPACT"
	Regular   SpotType = "REGULAR"
	Oversized SpotType = "OVERSIZED"
)

func (s SpotType) Valid() bool {
	return s == Compact || s == Regular || s == Oversized
}

func (s SpotType) Label() string {
	switch s {
	case Compact:
		return "Compact"
	case Regular:
		return "Regular"
	case Oversized:
		return "Oversized"
	}
	return string(s)
}

// Letter is the map letter drawn on a spot: C, R or L (large).
func (s SpotType) Letter() string {
	switch s {
	case Compact:
		return "C"
	case Regular:
		return "R"
	case Oversized:
		return "L"
	}
	return "?"
}

// HomeSpot returns the spot type a vehicle normally parks in (SRS 5.1).
func HomeSpot(v VehicleType) SpotType {
	switch v {
	case Motorcycle:
		return Compact
	case Truck:
		return Oversized
	default:
		return Regular
	}
}

// AllowedSpots lists spot types in allocation order (BR-1, BR-3).
// Without overflow only the home type is allowed; with it, larger types follow.
func AllowedSpots(v VehicleType, overflow bool) []SpotType {
	if !overflow {
		return []SpotType{HomeSpot(v)}
	}
	switch v {
	case Motorcycle:
		return []SpotType{Compact, Regular, Oversized}
	case Car:
		return []SpotType{Regular, Oversized}
	default:
		return []SpotType{Oversized}
	}
}

const (
	VehiclePending = "PENDING_VERIFICATION"
	VehicleActive  = "ACTIVE"
	VehicleRemoved = "REMOVED"

	SpotAvailable    = "AVAILABLE"
	SpotOccupied     = "OCCUPIED"
	SpotOutOfService = "OUT_OF_SERVICE"

	TicketActive         = "ACTIVE"
	TicketPaymentPending = "PAYMENT_PENDING"
	TicketClosed         = "CLOSED"
	TicketCancelled      = "CANCELLED"

	GateEntry = "ENTRY"
	GateExit  = "EXIT"

	ScanAdmitted           = "ADMITTED"
	ScanExitPendingPayment = "EXIT_PENDING_PAYMENT"
	ScanExitClosed         = "EXIT_CLOSED"
	ScanRejected           = "REJECTED"

	PaySimUPI    = "SIMULATED_UPI"
	PaySimCash   = "SIMULATED_CASH"
	PayWaived    = "WAIVED"
	PayManual    = "MANUAL"
	PayNoCharge  = "NO_CHARGE"
	SourceGate   = "GATE"
	SourceSim    = "SIMULATOR"
	SourceAdmin  = "ADMIN"
	EventEntered = "VehicleEntered"
	EventExited  = "VehicleExited"
)

// PaymentMethodLabel is how a payment method reads in emails ("UPI (simulated)").
func PaymentMethodLabel(m string) string {
	switch m {
	case PaySimUPI:
		return "UPI (simulated)"
	case PaySimCash:
		return "cash (simulated)"
	case PayWaived:
		return "waiver"
	case PayManual:
		return "manual payment at the office"
	case PayNoCharge:
		return "no charge"
	}
	return m
}
