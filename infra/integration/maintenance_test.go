package integration

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

func TestRetainedVersionsCannotStarveGarbageCollection(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), requiredEnv("PINELINK_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	scope := "gc-fairness-" + uuid.NewString()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	// More permanently retained old candidates than one maintenance batch.
	// Zero-byte fixtures need no physical object and cannot reference user data.
	for i := 0; i < 22; i++ {
		version := uuid.NewString()
		if _, err = tx.Exec(t.Context(), `INSERT INTO file_versions(id,scope_id,encryption_domain_id,hash_version,dedup_key,content_root,size_bytes,chunk_size,chunk_count,state,sealed_at,gc_after) VALUES($1,$2,'gc-fixture','sha256-v1',$3,'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',0,65536,0,'SEALED',now(),now()-interval '1 hour')`, version, scope, version); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(t.Context(), `INSERT INTO file_entries(id,version_id,owner_id,device_id,file_name,retention_class,state) VALUES($1,$2,$3,'fixture','empty','PERSISTENT','AVAILABLE')`, uuid.NewString(), version, scope); err != nil {
			t.Fatal(err)
		}
	}
	victim := uuid.NewString()
	if _, err = tx.Exec(t.Context(), `INSERT INTO file_versions(id,scope_id,encryption_domain_id,hash_version,dedup_key,content_root,size_bytes,chunk_size,chunk_count,state,sealed_at,gc_after) VALUES($1,$2,'gc-fixture','sha256-v1',$3,'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',0,65536,0,'SEALED',now(),now()-interval '1 second')`, victim, scope, victim); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var state string
		if err = pool.QueryRow(t.Context(), `SELECT state FROM file_versions WHERE id=$1`, victim).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "COLLECTED" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live old references starved GC: %s", state)
		}
		time.Sleep(100 * time.Millisecond)
	}
	var live int
	if err = pool.QueryRow(t.Context(), `SELECT count(*) FROM file_versions v WHERE v.scope_id=$1 AND v.id<>$2 AND v.state='SEALED'`, scope, victim).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 22 {
		t.Fatalf("GC touched retained versions: %d", live)
	}
}
