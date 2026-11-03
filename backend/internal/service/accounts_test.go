package service

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/simiooo/pinelink/backend/internal/accounts"
	"github.com/simiooo/pinelink/backend/internal/auth"
)

func TestSessionLogoutSerializesWithDurableDataCommit(t *testing.T) {
	s, _, _ := testStorage(t)
	if err := s.ConfigureAccounts(accounts.Config{Required: true, Origin: "https://pinelink.example", EncryptionKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}); err != nil {
		t.Fatal(err)
	}
	owner, session := "acct-"+uuid.NewString(), uuid.NewString()
	ctx := t.Context()
	if _, err := s.db.Exec(ctx, `INSERT INTO accounts(id) VALUES($1)`, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO devices(owner_id,device_id,status) VALUES($1,'test-device','ACTIVE')`, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO account_sessions(id,token_hash,account_id,device_id,credential_epoch,expires_at) VALUES($1,$2,$3,'test-device',1,now()+interval '1 hour')`, session, []byte{1}, owner); err != nil {
		t.Fatal(err)
	}
	identity := auth.WithIdentity(ctx, auth.Identity{OwnerID: owner, DeviceID: "test-device", SessionID: session})
	tx, err := s.beginAccountTx(identity)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	finished := make(chan error, 1)
	go func() {
		finished <- s.Accounts().Logout(context.Background(), accounts.Principal{SessionID: session, AccountID: owner, DeviceID: "test-device"})
	}()
	select {
	case err := <-finished:
		t.Fatalf("logout passed an active data commit: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if _, err = tx.Exec(ctx, `INSERT INTO scope_quotas(scope_id,quota_bytes) VALUES($1,100)`, owner); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("logout failed to drain")
	}
	if next, err := s.beginAccountTx(identity); !errors.Is(err, ErrUnauthorized) {
		if next != nil {
			next.Rollback(ctx)
		}
		t.Fatal("a revoked session started a late commit")
	}
	if _, err = s.GetQuota(identity, "another-scope"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("account could read another scope")
	}
	if _, err = s.PrepareUpload(identity, PrepareRequest{ScopeID: "another-scope"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("account could select an unbounded or foreign quota scope")
	}
}
