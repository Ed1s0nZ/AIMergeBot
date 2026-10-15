package platform

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"errors"
	"fmt"
)

var errLegacyCredentialState = errors.New("legacy repository credential state unavailable")
var errLegacyGenerationExhausted = errors.New("legacy repository credential generation exhausted")

type legacyRepositoryState struct {
	generation  int64
	key         []byte
	fingerprint []byte
}

func (legacyRepositoryState) Format(s fmt.State, _ rune) {
	fmt.Fprint(s, "[private repository credential state]")
}

func readLegacyRepositoryState(ctx context.Context, q queryRower) (legacyRepositoryState, error) {
	var s legacyRepositoryState
	var generationType, keyType, fingerprintType string
	var count int
	err := q.QueryRowContext(ctx, `SELECT generation,hmac_key,fingerprint,typeof(generation),typeof(hmac_key),typeof(fingerprint),(SELECT COUNT(*) FROM platform_legacy_repository_state) FROM platform_legacy_repository_state WHERE id=1`).Scan(&s.generation, &s.key, &s.fingerprint, &generationType, &keyType, &fingerprintType, &count)
	if err != nil || count != 1 || generationType != "integer" || keyType != "blob" || len(s.key) != 32 || s.generation < 0 || (s.generation == 0 && (fingerprintType != "null" || s.fingerprint != nil)) || (s.generation > 0 && (fingerprintType != "blob" || len(s.fingerprint) != 32)) {
		return legacyRepositoryState{}, errLegacyCredentialState
	}
	return s, nil
}

// The caller owns the immediate transaction so activation and cancellation of
// old tasks can commit together. This helper never creates or repairs state.
func activateLegacyRepositoryStateTx(ctx context.Context, tx *sql.Tx, material legacyRepositoryMaterial) (int64, bool, error) {
	s, err := readLegacyRepositoryState(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	fingerprint, err := material.fingerprint(s.key)
	if err != nil {
		return 0, false, err
	}
	if hmac.Equal(s.fingerprint, fingerprint) {
		return s.generation, false, nil
	}
	if s.generation == 1<<63-1 {
		return 0, false, errLegacyGenerationExhausted
	}
	next := s.generation + 1
	result, err := tx.ExecContext(ctx, `UPDATE platform_legacy_repository_state SET generation=?,fingerprint=?,updated_at=? WHERE id=1 AND generation=? AND hmac_key=? AND fingerprint IS ?`, next, fingerprint, now(), s.generation, s.key, s.fingerprint)
	if err != nil {
		return 0, false, errLegacyCredentialState
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return 0, false, errLegacyCredentialState
	}
	return next, true, nil
}

// This is a read-only check, not a credential activation or an ACL decision.
func validateLegacyRepositoryState(ctx context.Context, q queryRower, material legacyRepositoryMaterial, generation int64) error {
	s, err := readLegacyRepositoryState(ctx, q)
	if err != nil {
		return err
	}
	fingerprint, err := material.fingerprint(s.key)
	if err != nil {
		return err
	}
	if generation <= 0 || s.generation != generation || !hmac.Equal(s.fingerprint, fingerprint) {
		return errLegacyCredentialState
	}
	return nil
}
