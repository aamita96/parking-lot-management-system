package email

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// fakeResend stands in for api.resend.com and records what the sender sent.
type fakeResend struct {
	status         int
	body           string
	idempotencyKey string
	payload        map[string]any
}

func (f *fakeResend) start(t *testing.T) *ResendSender {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.idempotencyKey = r.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(r.Body).Decode(&f.payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(server.Close)
	sender := NewResend("re_test", "PLMS <onboarding@resend.dev>", "")
	sender.client.BaseURL, _ = url.Parse(server.URL + "/")
	return sender
}

// Code emails have no idempotency key. This used to crash inside the Resend library (HTTP 500).
func TestResendSendWithoutIdempotencyKey(t *testing.T) {
	fake := &fakeResend{status: 200, body: `{"id":"email-1"}`}
	sender := fake.start(t)

	id, err := sender.Send(context.Background(), Message{To: "owner@example.com", Subject: "Your PLMS code: 123456", HTML: "<p>123456</p>"})
	if err != nil || id != "email-1" {
		t.Fatalf("Send = %q, %v", id, err)
	}
	if fake.idempotencyKey != "" {
		t.Errorf("unexpected Idempotency-Key header %q", fake.idempotencyKey)
	}
	if fake.payload["subject"] != "Your PLMS code: 123456" {
		t.Errorf("payload = %v", fake.payload)
	}
}

// Entry/exit emails pass the event id so Resend never sends the same email twice (FR-30).
func TestResendSendWithIdempotencyKey(t *testing.T) {
	fake := &fakeResend{status: 200, body: `{"id":"email-2"}`}
	sender := fake.start(t)

	if _, err := sender.Send(context.Background(), Message{To: "owner@example.com", Subject: "Checked in", HTML: "x", IdempotencyKey: "event-1-0"}); err != nil {
		t.Fatal(err)
	}
	if fake.idempotencyKey != "event-1-0" {
		t.Errorf("Idempotency-Key = %q, want event-1-0", fake.idempotencyKey)
	}
}

// Resend's answers are turned into the error kinds the app reacts to (retry, defer, give up).
func TestResendErrorsAreClassified(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"daily quota", 429, `{"name":"daily_quota_exceeded","message":"You have reached your daily email sending quota."}`, ErrQuota},
		{"rate limit", 429, `{"name":"rate_limit_exceeded","message":"Too many requests."}`, ErrRateLimit},
		{"unverified domain", 403, `{"name":"validation_error","message":"You can only send testing emails to your own email address. To send to other recipients, please verify a domain."}`, ErrPermanent},
		{"server error", 500, `{"message":"Internal server error"}`, ErrTransient},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeResend{status: c.status, body: c.body}
			sender := fake.start(t)
			_, err := sender.Send(context.Background(), Message{To: "someone@example.com", Subject: "s", HTML: "x"})
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}
