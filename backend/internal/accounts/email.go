package accounts

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type EmailChallenge struct {
	ID         string    `json:"id"`
	Purpose    string    `json:"purpose"`
	Generation int       `json:"generation"`
	Expires    time.Time `json:"expires_at"`
	ResendAt   time.Time `json:"resend_at"`
	Delivery   string    `json:"delivery_state"`
}
type emailRow struct {
	EmailChallenge
	RequestID, Email   string
	AccountID          *string
	Version            *int64
	Password           *string
	CodeHash, LinkHash []byte
	Attempts           int
	Consumed           *time.Time
}

const emailFields = `id::text,purpose,generation,expires_at,resend_at,request_id::text,email,account_id,credential_version,pending_password_phc,code_hash,link_hash,attempts,consumed_at`

func scanEmail(row pgx.Row) (emailRow, error) {
	var e emailRow
	err := row.Scan(&e.ID, &e.Purpose, &e.Generation, &e.Expires, &e.ResendAt, &e.RequestID, &e.Email, &e.AccountID, &e.Version, &e.Password, &e.CodeHash, &e.LinkHash, &e.Attempts, &e.Consumed)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, fault(410, "LINK_EXPIRED")
	}
	return e, err
}
func (s *Service) delivery(ctx context.Context, e emailRow) EmailChallenge {
	result := e.EmailChallenge
	if e.Purpose == "reset" {
		result.Delivery = "conditional"
		return result
	}
	var state string
	err := s.db.QueryRow(ctx, `SELECT status FROM account_mail_outbox WHERE challenge_id=$1 AND generation=$2`, e.ID, e.Generation).Scan(&state)
	if err != nil {
		state = "pending"
	}
	result.Delivery = state
	return result
}
func (s *Service) emailTokens(e emailRow) (string, string, []byte, []byte) {
	n, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		panic(err)
	}
	code := fmt.Sprintf("%08d", n.Int64())
	link := randomToken(16)
	return code, link, s.mac("code", fmt.Sprintf("%s/%d/%s", e.ID, e.Generation, code)), s.mac("link", link)
}
func (s *Service) enqueueMail(ctx context.Context, tx pgx.Tx, e emailRow, code, link string) error {
	if e.Purpose == "reset" && e.AccountID == nil {
		return nil
	}
	body, _ := json.Marshal(Mail{To: e.Email, Purpose: e.Purpose, Code: code, Link: s.config.Origin + "/a/" + link})
	_, err := tx.Exec(ctx, `INSERT INTO account_mail_outbox(id,challenge_id,generation,payload_cipher) VALUES($1,$2,$3,$4)`, uuid.NewString(), e.ID, e.Generation, s.seal(body))
	return err
}
func (s *Service) StartEmail(ctx context.Context, r Request, purpose, email, password string) (EmailChallenge, error) {
	var empty EmailChallenge
	if s.sender == nil {
		return empty, fault(503, "MAIL_UNAVAILABLE")
	}
	if purpose != "signup" && purpose != "reset" {
		return empty, fault(400, "AUTH_INVALID_REQUEST")
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return empty, err
	}
	if err = s.Rate(ctx, "mail/"+email, 8, 24*time.Hour); err != nil {
		return empty, err
	}
	var phc *string
	if purpose == "signup" {
		value, err := s.passwordHash(ctx, password)
		if err != nil {
			return empty, err
		}
		phc = &value
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	current, err := s.lockRequest(ctx, tx, r)
	if err != nil {
		return empty, err
	}
	if current.Status != "PENDING" {
		return empty, fault(409, "AUTH_STATE_CHANGED")
	}
	old, err := scanEmail(tx.QueryRow(ctx, `SELECT `+emailFields+` FROM account_email_challenges WHERE request_id=$1 AND purpose=$2`, r.ID, purpose))
	if err == nil {
		if old.Email != email {
			return empty, fault(409, "AUTH_STATE_CHANGED")
		}
		var state string
		_ = tx.QueryRow(ctx, `SELECT status FROM account_mail_outbox WHERE challenge_id=$1 AND generation=$2`, old.ID, old.Generation).Scan(&state)
		old.Delivery = state
		if old.Purpose == "reset" {
			old.Delivery = "conditional"
		}
		return old.EmailChallenge, nil
	}
	if _, ok := err.(*Error); !ok {
		return empty, err
	}
	e := emailRow{EmailChallenge: EmailChallenge{ID: uuid.NewString(), Purpose: purpose, Generation: 1, Expires: s.now().Add(15 * time.Minute), ResendAt: s.now().Add(time.Minute), Delivery: "pending"}, RequestID: r.ID, Email: email, Password: phc}
	if purpose == "reset" {
		var account string
		var version int64
		err = tx.QueryRow(ctx, `SELECT account_id,version FROM account_passwords WHERE email=$1`, email).Scan(&account, &version)
		if err == nil {
			e.AccountID = &account
			e.Version = &version
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return empty, err
		}
		e.Delivery = "conditional"
	}
	code, link, codeHash, linkHash := s.emailTokens(e)
	_, err = tx.Exec(ctx, `INSERT INTO account_email_challenges(id,request_id,purpose,email,account_id,credential_version,pending_password_phc,code_hash,link_hash,expires_at,resend_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, e.ID, r.ID, purpose, email, e.AccountID, e.Version, phc, codeHash, linkHash, e.Expires, e.ResendAt)
	if err != nil {
		return empty, err
	}
	if err = s.enqueueMail(ctx, tx, e, code, link); err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return e.EmailChallenge, nil
}
func (s *Service) EmailStatus(ctx context.Context, r Request, id string) (EmailChallenge, error) {
	e, err := scanEmail(s.db.QueryRow(ctx, `SELECT `+emailFields+` FROM account_email_challenges WHERE id=$1 AND request_id=$2`, id, r.ID))
	if err != nil {
		return EmailChallenge{}, err
	}
	return s.delivery(ctx, e), nil
}
func (s *Service) Resend(ctx context.Context, r Request, id string, generation int) (EmailChallenge, error) {
	if s.sender == nil {
		return EmailChallenge{}, fault(503, "MAIL_UNAVAILABLE")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EmailChallenge{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = s.lockRequest(ctx, tx, r); err != nil {
		return EmailChallenge{}, err
	}
	e, err := scanEmail(tx.QueryRow(ctx, `SELECT `+emailFields+` FROM account_email_challenges WHERE id=$1 AND request_id=$2 FOR UPDATE`, id, r.ID))
	if err != nil {
		return EmailChallenge{}, err
	}
	if e.Consumed != nil {
		return EmailChallenge{}, fault(410, "LINK_EXPIRED")
	}
	if e.Generation != generation {
		var state string
		_ = tx.QueryRow(ctx, `SELECT status FROM account_mail_outbox WHERE challenge_id=$1 AND generation=$2`, e.ID, e.Generation).Scan(&state)
		if state == "" {
			state = "pending"
		}
		e.Delivery = state
		if e.Purpose == "reset" {
			e.Delivery = "conditional"
		}
		return e.EmailChallenge, nil
	}
	if s.now().Before(e.ResendAt) {
		return EmailChallenge{}, fault(429, "RESEND_WAIT")
	}
	if err = s.rate(ctx, tx, "mail/"+e.Email, 8, 24*time.Hour); err != nil {
		return EmailChallenge{}, err
	}
	e.Generation++
	e.Expires = s.now().Add(15 * time.Minute)
	e.ResendAt = s.now().Add(time.Minute)
	code, link, ch, lh := s.emailTokens(e)
	_, err = tx.Exec(ctx, `UPDATE account_email_challenges SET generation=$2,attempts=0,code_hash=$3,link_hash=$4,expires_at=$5,resend_at=$6 WHERE id=$1`, e.ID, e.Generation, ch, lh, e.Expires, e.ResendAt)
	if err != nil {
		return EmailChallenge{}, err
	}
	if err = s.enqueueMail(ctx, tx, e, code, link); err != nil {
		return EmailChallenge{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return EmailChallenge{}, err
	}
	e.Delivery = "pending"
	if e.Purpose == "reset" {
		e.Delivery = "conditional"
	}
	return e.EmailChallenge, nil
}

type Verification struct {
	State      string `json:"state"`
	ResetGrant string `json:"reset_grant,omitempty"`
	AccountID  string `json:"account_id,omitempty"`
}
type VerifyInput struct {
	ChallengeID string `json:"challenge_id"`
	Generation  int    `json:"generation"`
	Code        string `json:"code"`
	Link        string `json:"link"`
	NewPassword string `json:"new_password"`
}

func (s *Service) InspectLink(ctx context.Context, token string) (map[string]any, error) {
	if len(token) != 22 {
		return nil, fault(410, "LINK_EXPIRED")
	}
	e, err := scanEmail(s.db.QueryRow(ctx, `SELECT `+emailFields+` FROM account_email_challenges WHERE link_hash=$1`, s.mac("link", token)))
	if err != nil {
		return nil, err
	}
	if e.Consumed != nil || !s.now().Before(e.Expires) || e.Attempts >= 5 {
		return nil, fault(410, "LINK_EXPIRED")
	}
	return map[string]any{"purpose": e.Purpose, "expires_at": e.Expires, "challenge_id": e.ID, "request_id": e.RequestID}, nil
}
func (s *Service) VerifyEmail(ctx context.Context, r Request, in VerifyInput) (Verification, error) {
	var result Verification
	var freshPassword *string
	if in.NewPassword != "" {
		value, err := s.passwordHash(ctx, in.NewPassword)
		if err != nil {
			return result, err
		}
		freshPassword = &value
	}
	original := r.ID
	if in.Link != "" {
		info, err := s.InspectLink(ctx, in.Link)
		if err != nil {
			return result, err
		}
		original = info["request_id"].(string)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	// Cross-device verification and resend take request locks in the same order.
	rows, err := tx.Query(ctx, `SELECT id FROM account_auth_requests WHERE id IN ($1,$2) ORDER BY id FOR UPDATE`, r.ID, original)
	if err != nil {
		return result, err
	}
	rows.Close()
	current, err := s.lockRequest(ctx, tx, r)
	if err != nil {
		return result, err
	}
	if current.Status != "PENDING" {
		return result, fault(409, "AUTH_STATE_CHANGED")
	}
	var e emailRow
	if in.Link != "" {
		if len(in.Link) != 22 {
			return result, fault(410, "LINK_EXPIRED")
		}
		e, err = scanEmail(tx.QueryRow(ctx, `SELECT `+emailFields+` FROM account_email_challenges WHERE link_hash=$1 FOR UPDATE`, s.mac("link", in.Link)))
	} else {
		e, err = scanEmail(tx.QueryRow(ctx, `SELECT `+emailFields+` FROM account_email_challenges WHERE id=$1 AND request_id=$2 FOR UPDATE`, in.ChallengeID, r.ID))
	}
	if err != nil {
		return result, err
	}
	if e.Consumed != nil || !s.now().Before(e.Expires) || e.Attempts >= 5 {
		return result, fault(410, "LINK_EXPIRED")
	}
	if in.Link == "" {
		if e.Generation != in.Generation {
			return result, fault(409, "CODE_REPLACED")
		}
		actual := s.mac("code", fmt.Sprintf("%s/%d/%s", e.ID, e.Generation, in.Code))
		if len(in.Code) != 8 || subtle.ConstantTimeCompare(actual, e.CodeHash) != 1 {
			_, err = tx.Exec(ctx, `UPDATE account_email_challenges SET attempts=attempts+1 WHERE id=$1`, e.ID)
			if err == nil {
				err = tx.Commit(ctx)
			}
			if err != nil {
				return result, err
			}
			return result, fault(400, "CODE_INVALID")
		}
	}
	if e.RequestID != r.ID && e.Purpose == "signup" && freshPassword == nil {
		return result, fault(400, "NEW_PASSWORD_REQUIRED")
	}
	if e.Purpose == "signup" {
		password := e.Password
		if e.RequestID != r.ID {
			password = freshPassword
		}
		if password == nil {
			return result, fault(400, "NEW_PASSWORD_REQUIRED")
		}
		// A verified existing email never promotes a pending attacker's password.
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_passwords WHERE email=$1)`, e.Email).Scan(&exists); err != nil {
			return result, err
		}
		if exists {
			result.State = "existing_account"
		} else {
			// Serialize first signup per normalized email, without retaining a lock across KDF work.
			if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, e.Email); err != nil {
				return result, err
			}
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_passwords WHERE email=$1)`, e.Email).Scan(&exists); err != nil {
				return result, err
			}
			if exists {
				result.State = "existing_account"
			} else {
				id, err := s.enrollAccount(ctx, tx, e.Email, "")
				if err != nil {
					return result, err
				}
				if _, err = tx.Exec(ctx, `INSERT INTO account_passwords(email,account_id,password_phc) VALUES($1,$2,$3)`, e.Email, id, *password); err != nil {
					return result, err
				}
				if _, err = tx.Exec(ctx, `UPDATE account_auth_requests SET account_id=$2,credential_epoch=1,status='READY',ready_until=now()+interval '5 minutes' WHERE id=$1`, r.ID, id); err != nil {
					return result, err
				}
				result.State = "ready"
				result.AccountID = id
			}
		}
	} else {
		if e.AccountID == nil || e.Version == nil {
			return result, denied()
		}
		grant := randomToken(32)
		result.State = "reset"
		result.ResetGrant = grant
		if _, err = tx.Exec(ctx, `UPDATE account_email_challenges SET reset_hash=$2,request_id=$3 WHERE id=$1`, e.ID, s.mac("reset", grant), r.ID); err != nil {
			return result, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE account_email_challenges SET consumed_at=now(),pending_password_phc=NULL WHERE id=$1`, e.ID)
	if err != nil {
		return result, err
	}
	// Cross-device proof does not make the initiating request ready.
	if e.RequestID != r.ID {
		if _, err = tx.Exec(ctx, `UPDATE account_auth_requests SET status='CANCELLED' WHERE id=$1 AND status='PENDING'`, e.RequestID); err != nil {
			return result, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}
func (s *Service) ResetPassword(ctx context.Context, r Request, grant, password string) error {
	phc, err := s.passwordHash(ctx, password)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = s.lockRequest(ctx, tx, r); err != nil {
		return err
	}
	var account string
	var version int64
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text,account_id,credential_version FROM account_email_challenges WHERE request_id=$1 AND purpose='reset' AND reset_hash=$2 AND consumed_at IS NOT NULL AND expires_at>now() FOR UPDATE`, r.ID, s.mac("reset", grant)).Scan(&id, &account, &version)
	if err != nil {
		return denied()
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE account_passwords SET password_phc=$2,version=version+1,updated_at=now() WHERE account_id=$1 AND version=$3`, account, phc, version)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fault(409, "RESET_REPLACED")
	}
	if _, err = tx.Exec(ctx, `UPDATE accounts SET credential_epoch=credential_epoch+1 WHERE id=$1`, account); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1`, account); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_email_challenges SET reset_hash=NULL WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_auth_requests SET status='CANCELLED' WHERE id=$1`, r.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
