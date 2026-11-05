// Package email sends transactional email through Resend (SRS 9) and renders the templates.
package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/resend/resend-go/v2"

	"plms/internal/domain"
)

type Message struct {
	To             string
	Subject        string
	HTML           string
	Text           string
	IdempotencyKey string
	Tags           map[string]string
}

// Sender delivers one message and returns the provider message ID.
type Sender interface {
	Send(ctx context.Context, m Message) (string, error)
}

// Error classes the dispatcher acts on (SRS 9.5).
var (
	ErrQuota     = errors.New("email quota exceeded")      // DEFERRED, retry in 1 h
	ErrRateLimit = errors.New("email rate limited")        // retry soon
	ErrTransient = errors.New("email temporarily failing") // backoff retry
	ErrPermanent = errors.New("email rejected")            // FAILED
)

type ResendSender struct {
	client  *resend.Client
	from    string
	replyTo string
}

func NewResend(apiKey, from, replyTo string) *ResendSender {
	return &ResendSender{client: resend.NewClient(apiKey), from: from, replyTo: replyTo}
}

func (s *ResendSender) Send(ctx context.Context, m Message) (string, error) {
	req := &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{m.To},
		Subject: m.Subject,
		Html:    m.HTML,
		Text:    m.Text,
		ReplyTo: s.replyTo,
	}
	for k, v := range m.Tags {
		req.Tags = append(req.Tags, resend.Tag{Name: k, Value: v})
	}
	// Always pass real options, even with an empty key (code emails have none). A nil
	// *SendEmailOptions would look non-nil inside the library and crash it with a nil-pointer panic.
	// An empty key is simply not sent to Resend.
	opts := &resend.SendEmailOptions{IdempotencyKey: m.IdempotencyKey}
	resp, err := s.client.Emails.SendWithOptions(ctx, req, opts)
	if err != nil {
		return "", classify(err)
	}
	return resp.Id, nil
}

func classify(err error) error {
	var rl *resend.RateLimitError
	if errors.As(err, &rl) {
		msg := strings.ToLower(rl.Message)
		if strings.Contains(msg, "quota") {
			return fmt.Errorf("%w: %s", ErrQuota, rl.Message)
		}
		return fmt.Errorf("%w: %s", ErrRateLimit, rl.Message)
	}
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrTransient, err)
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "quota"):
		return fmt.Errorf("%w: %v", ErrQuota, err)
	case strings.Contains(msg, "validation") || strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "not allowed") || strings.Contains(msg, "verify a domain"):
		return fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	return fmt.Errorf("%w: %v", ErrTransient, err)
}

// DevSender logs messages instead of sending them (EMAIL_ENABLED=false) and keeps the last few.
type DevSender struct {
	mu   sync.Mutex
	last []Message
	seq  int
}

func NewDevSender() *DevSender { return &DevSender{} }

func (d *DevSender) Send(ctx context.Context, m Message) (string, error) {
	d.mu.Lock()
	d.seq++
	id := fmt.Sprintf("dev-%d", d.seq)
	d.last = append(d.last, m)
	if len(d.last) > 50 {
		d.last = d.last[1:]
	}
	d.mu.Unlock()
	slog.InfoContext(ctx, "email (dev mode, not sent)", "to", domain.MaskEmail(m.To), "subject", redactCode(m.Subject), "id", id)
	return id, nil
}

// redactCode hides the code in "Your PLMS code: 483920" when logging.
func redactCode(subject string) string {
	if strings.HasPrefix(subject, "Your PLMS code") {
		return "Your PLMS code: ******"
	}
	return subject
}
