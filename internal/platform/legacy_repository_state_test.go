package platform

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func credentialMaterialFixture(t *testing.T, token string) legacyRepositoryMaterial {
	t.Helper()
	m, err := newLegacyRepositoryMaterial(token, "https://gitlab.example/prefix", noCredentialProxy)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func activateCredentialFixture(ctx context.Context, s *Store, m legacyRepositoryMaterial) (int64, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	g, changed, err := activateLegacyRepositoryStateTx(ctx, tx, m)
	if err != nil {
		return 0, false, err
	}
	return g, changed, tx.Commit()
}

func TestLegacyCredentialGenerationABAAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, b := credentialMaterialFixture(t, "PRIVATE-A"), credentialMaterialFixture(t, "PRIVATE-B")
	if err := validateLegacyRepositoryState(ctx, s.DB, a, 0); !errors.Is(err, errLegacyCredentialState) {
		t.Fatal("unactivated state authorized", err)
	}
	for i, item := range []struct {
		material legacyRepositoryMaterial
		want     int64
		changed  bool
	}{{a, 1, true}, {a, 1, false}, {b, 2, true}, {a, 3, true}} {
		g, changed, err := activateCredentialFixture(ctx, s, item.material)
		if err != nil || g != item.want || changed != item.changed {
			t.Fatal("activation", i, g, changed, err)
		}
		if err = validateLegacyRepositoryState(ctx, s.DB, item.material, g); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateLegacyRepositoryState(ctx, s.DB, a, 1); !errors.Is(err, errLegacyCredentialState) {
		t.Fatal("old A generation revived", err)
	}
	if err := validateLegacyRepositoryState(ctx, s.DB, b, 3); !errors.Is(err, errLegacyCredentialState) {
		t.Fatal("different credentials authorized", err)
	}
	before, err := readLegacyRepositoryState(ctx, s.DB)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g, changed, err := activateLegacyRepositoryStateTx(ctx, tx, b); err != nil || g != 4 || !changed {
		tx.Rollback()
		t.Fatal("transactional activation", g, changed, err)
	}
	// A later cancellation failure must also roll back credential activation.
	if _, err = tx.ExecContext(ctx, `INSERT INTO nonexistent_cancellation_table VALUES(1)`); err == nil {
		tx.Rollback()
		t.Fatal("expected downstream transaction failure")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	after, err := readLegacyRepositoryState(ctx, s.DB)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("activation escaped caller rollback", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_legacy_repository_state SET generation=9223372036854775807`); err != nil {
		t.Fatal(err)
	}
	if g, changed, err := activateCredentialFixture(ctx, s, a); err != nil || g != 1<<63-1 || changed {
		t.Fatal("unchanged exhausted state rejected", g, changed, err)
	}
	if _, _, err = activateCredentialFixture(ctx, s, b); !errors.Is(err, errLegacyGenerationExhausted) {
		t.Fatal("exhausted generation changed", err)
	}
}

func TestLegacyCredentialActivationSerializesConnections(t *testing.T) {
	s := testStore(t)
	var seq int
	var name, path string
	if err := s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	other, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	m := credentialMaterialFixture(t, "PRIVATE-CONCURRENT")
	type observation struct {
		generation int64
		changed    bool
		err        error
	}
	results := make(chan observation, 8)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		store := []*Store{s, other}[i%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			g, changed, err := activateCredentialFixture(ctx, store, m)
			results <- observation{g, changed, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	changes := 0
	for v := range results {
		if v.err != nil || v.generation != 1 {
			t.Fatal("concurrent activation", v.generation, v.err)
		}
		if v.changed {
			changes++
		}
	}
	if changes != 1 {
		t.Fatal("same material rotated more than once", changes)
	}
	if err := validateLegacyRepositoryState(ctx, other.DB, m, 1); err != nil {
		t.Fatal("other connection cannot validate committed state", err)
	}
	next := credentialMaterialFixture(t, "PRIVATE-ROTATED")
	if g, changed, err := activateCredentialFixture(ctx, other, next); err != nil || g != 2 || !changed {
		t.Fatal("other connection cannot rotate credentials", g, changed, err)
	}
	if err := validateLegacyRepositoryState(ctx, s.DB, m, 1); !errors.Is(err, errLegacyCredentialState) {
		t.Fatal("old connection authorized stale credentials", err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if g, changed, err := activateCredentialFixture(ctx, reopened, next); err != nil || g != 2 || changed {
		t.Fatal("unchanged restart rotated credentials", g, changed, err)
	}
}

func TestLegacyCredentialCorruptionIsNotReinitialized(t *testing.T) {
	for _, mutation := range []string{
		`DELETE FROM platform_legacy_repository_state`,
		`UPDATE platform_legacy_repository_state SET hmac_key=X'00'`,
		`UPDATE platform_legacy_repository_state SET hmac_key='PRIVATE-INVALID-KEY'`,
		`UPDATE platform_legacy_repository_state SET generation='PRIVATE-BAD-GENERATION'`,
		`UPDATE platform_legacy_repository_state SET generation=0`,
		`UPDATE platform_legacy_repository_state SET fingerprint=NULL`,
		`UPDATE platform_legacy_repository_state SET fingerprint='12345678901234567890123456789012'`,
		`INSERT INTO platform_legacy_repository_state SELECT 2,generation,hmac_key,fingerprint,updated_at FROM platform_legacy_repository_state`,
	} {
		t.Run(mutation, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			m := credentialMaterialFixture(t, "PRIVATE-VALID")
			if _, _, err := activateCredentialFixture(ctx, s, m); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			var before string
			const manifest = `SELECT COALESCE(group_concat(id||':'||typeof(generation)||':'||quote(generation)||':'||typeof(hmac_key)||':'||quote(hmac_key)||':'||typeof(fingerprint)||':'||quote(fingerprint)||':'||updated_at),'') FROM platform_legacy_repository_state ORDER BY id`
			if err := s.DB.QueryRow(manifest).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, _, err := activateCredentialFixture(ctx, s, m); !errors.Is(err, errLegacyCredentialState) {
				t.Fatal("corrupt state activated", err)
			}
			if err := validateLegacyRepositoryState(ctx, s.DB, m, 1); !errors.Is(err, errLegacyCredentialState) {
				t.Fatal("corrupt state authorized", err)
			}
			var after string
			if err := s.DB.QueryRow(manifest).Scan(&after); err != nil || before != after {
				t.Fatal("corrupt state changed or repaired", err)
			}
		})
	}
}
