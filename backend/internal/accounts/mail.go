package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type Mail struct {
	To      string `json:"to"`
	Purpose string `json:"purpose"`
	Code    string `json:"code"`
	Link    string `json:"link"`
}
type MailSender interface {
	Send(context.Context, string, Mail) (string, error)
}
type ResendSender struct {
	Key, From, ReplyTo string
	Client             *http.Client
}

func (r *ResendSender) Send(ctx context.Context, id string, m Mail) (string, error) {
	subject := "Verify your Pinelink email"
	if m.Purpose == "reset" {
		subject = "Reset your Pinelink password"
	}
	text := fmt.Sprintf("%s\n\n%s\n\nCode: %s\n\nExpires in 15 minutes. If you did not request this, ignore this message.", subject, m.Link, m.Code)
	content := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;color:#101010;max-width:480px;margin:auto;padding:32px 20px"><h1 style="font-size:24px">Pinelink</h1><p>%s</p><p style="margin:32px 0"><a href="%s" style="display:inline-block;background:#0866ff;color:white;border-radius:24px;padding:14px 24px;text-decoration:none">Continue</a></p><p style="font-size:24px;letter-spacing:4px">%s</p><p>Expires in 15 minutes. If you did not request this, ignore this message.</p></div>`, html.EscapeString(subject), html.EscapeString(m.Link), html.EscapeString(m.Code))
	payload := map[string]any{"from": r.From, "to": []string{m.To}, "subject": subject, "html": content, "text": text}
	if r.ReplyTo != "" {
		payload["reply_to"] = r.ReplyTo
	}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return "", fault(503, "MAIL_UNAVAILABLE")
	}
	request.Header.Set("Authorization", "Bearer "+r.Key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "pinelink/"+id)
	response, err := r.Client.Do(request)
	if err != nil {
		return "", fault(503, "MAIL_UNAVAILABLE")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fault(503, "MAIL_UNAVAILABLE")
	}
	var sent struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&sent) != nil || sent.ID == "" {
		return "", fault(503, "MAIL_UNAVAILABLE")
	}
	return sent.ID, nil
}
func (s *Service) RunMail(ctx context.Context) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	cleanupAt := s.now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if !s.now().Before(cleanupAt) {
				s.cleanAuth(ctx)
				cleanupAt = s.now().Add(time.Minute)
			}
			for i := 0; i < 4; i++ {
				worked, _ := s.mailOne(ctx)
				if !worked {
					break
				}
			}
		}
	}
}
func (s *Service) mailOne(ctx context.Context) (bool, error) {
	if s.sender == nil {
		return false, nil
	}
	var id, challenge string
	var generation, attempts int
	var encrypted []byte
	// Lease acquisition is atomic; provider I/O never owns a database row lock.
	err := s.db.QueryRow(ctx, `UPDATE account_mail_outbox SET lease_until=now()+interval '30 seconds',attempts=attempts+1 WHERE id=(SELECT o.id FROM account_mail_outbox o JOIN account_email_challenges c ON c.id=o.challenge_id WHERE o.status IN ('pending','retry') AND o.next_attempt_at<=now() AND (o.lease_until IS NULL OR o.lease_until<now()) AND o.generation=c.generation AND c.consumed_at IS NULL AND c.expires_at>now() ORDER BY o.next_attempt_at FOR UPDATE OF o SKIP LOCKED LIMIT 1) RETURNING id::text,challenge_id::text,generation,payload_cipher,attempts`).Scan(&id, &challenge, &generation, &encrypted, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	plain, err := s.open(encrypted)
	var mail Mail
	if err == nil {
		err = json.Unmarshal(plain, &mail)
	}
	var current bool
	if err == nil {
		err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_email_challenges WHERE id=$1 AND generation=$2 AND consumed_at IS NULL AND expires_at>now())`, challenge, generation).Scan(&current)
	}
	var providerID string
	if err == nil && current {
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		providerID, err = s.sender.Send(sendCtx, id, mail)
		cancel()
	}
	state := "sent"
	if !current {
		state = "superseded"
	} else if err != nil {
		state = "retry"
		if attempts >= 3 {
			state = "failed"
		}
	}
	_, updateErr := s.db.Exec(ctx, `UPDATE account_mail_outbox SET status=$3,provider_id=NULLIF($4,''),lease_until=NULL,next_attempt_at=now()+interval '10 seconds',updated_at=now(),payload_cipher=CASE WHEN $3 IN ('sent','failed','superseded') THEN '\x'::bytea ELSE payload_cipher END WHERE id=$1 AND generation=$2`, id, generation, state, providerID)
	return true, updateErr
}
