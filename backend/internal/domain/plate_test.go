package domain

import "testing"

func TestNormalisePlate(t *testing.T) {
	cases := map[string]string{
		"ka 01-ab 1234":  "KA01AB1234",
		"  MH12XY9999 ":  "MH12XY9999",
		"22 bh 1234 ab":  "22BH1234AB",
		"dl-3c-ab-1234":  "DL3CAB1234",
		"":               "",
	}
	for in, want := range cases {
		if got := NormalisePlate(in); got != want {
			t.Errorf("NormalisePlate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidPlate(t *testing.T) {
	valid := []string{"KA01AB1234", "MH12XY9999", "DL3CAB1234", "KA011234", "22BH1234AB", "21BH0001A", "TN9Z1234"}
	invalid := []string{"", "KA01AB123", "K01AB1234", "KA01ABCD1234", "22BH123AB", "22BH1234ABC", "HELLO", "1234KA01"}
	for _, p := range valid {
		if !ValidPlate(p) {
			t.Errorf("expected %q to be valid", p)
		}
	}
	for _, p := range invalid {
		if ValidPlate(p) {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}

func TestNormaliseMobile(t *testing.T) {
	ok := map[string]string{
		"9876543210":       "+919876543210",
		"+91 98765 43210":  "+919876543210",
		"919876543210":     "+919876543210",
		"09876543210":      "+919876543210",
		"6000000000":       "+916000000000",
	}
	for in, want := range ok {
		got, valid := NormaliseMobile(in)
		if !valid || got != want {
			t.Errorf("NormaliseMobile(%q) = %q,%v want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"5876543210", "98765", "+1 2025550123", "abcdefghij"} {
		if _, valid := NormaliseMobile(in); valid {
			t.Errorf("expected %q invalid", in)
		}
	}
}

func TestEmailAndMasks(t *testing.T) {
	if !ValidEmail("owner@example.com") || ValidEmail("owner@localhost") || ValidEmail("Name <a@b.com>") || ValidEmail("") {
		t.Error("ValidEmail mismatch")
	}
	if got := MaskEmail("owner@example.com"); got != "o***@example.com" {
		t.Errorf("MaskEmail = %q", got)
	}
	if got := MaskEmail("nope"); got != "***" {
		t.Errorf("MaskEmail = %q", got)
	}
	if got := MaskMobile("+919876543210"); got != "+91******3210" {
		t.Errorf("MaskMobile = %q", got)
	}
	if got := MaskMobile("12345"); got != "****" {
		t.Errorf("MaskMobile short = %q", got)
	}
	if !ValidRegistrationID("KA-01-RC-99") || ValidRegistrationID("bad id!") {
		t.Error("ValidRegistrationID mismatch")
	}
}
