package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

var errRepositoryHistoryCollision = errors.New("native project identity overlaps legacy run history; offline repair required")

func decodeIdentityHistory(raw string, revision int64) (RepositoryBinding, bool) {
	var b RepositoryBinding
	if revision <= 0 || !utf8.ValidString(raw) || uniqueScopeJSON([]byte(raw)) != nil || json.Unmarshal([]byte(raw), &b) != nil || b.Revision != revision {
		return b, false
	}
	if _, err := scopeObject(json.RawMessage(raw), "revision", "provider", "api_origin", "remote_id", "full_name", "integration_id"); err != nil {
		return b, false
	}
	valid, err := validateRepositoryBinding(b)
	return b, err == nil && b.APIOrigin == valid.APIOrigin
}

func migrationCurrentBinding(ctx context.Context, tx *sql.Tx, project int) (RepositoryBinding, bool, error) {
	var revision, provider, origin, remote, raw any
	err := tx.QueryRowContext(ctx, `SELECT revision,provider,api_origin,remote_id,CASE WHEN length(CAST(binding_json AS BLOB))<=4096 THEN binding_json ELSE '' END FROM platform_project_repositories WHERE project_id=?`, project).Scan(&revision, &provider, &origin, &remote, &raw)
	if err == sql.ErrNoRows {
		return RepositoryBinding{}, false, nil
	}
	if err != nil {
		return RepositoryBinding{}, false, err
	}
	r, rOK := revision.(int64)
	p, pOK := provider.(string)
	o, oOK := origin.(string)
	id, idOK := remote.(int64)
	body, bodyOK := raw.(string)
	if !rOK || !pOK || !oOK || !idOK || !bodyOK {
		return RepositoryBinding{}, false, nil
	}
	b, valid := decodeIdentityHistory(body, r)
	return b, valid && b.Provider == p && b.APIOrigin == o && b.RemoteID == id, nil
}

// Migration validates identity evidence without loading tokens or granting ACLs.
func classifyRepositoryIdentity(ctx context.Context, tx *sql.Tx, project int) (string, string, error) {
	var explicit bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_project_repositories WHERE project_id=?) OR EXISTS(SELECT 1 FROM platform_repository_binding_history WHERE project_id=?) OR EXISTS(SELECT 1 FROM platform_repository_project_receipts WHERE project_id=?)`, project, project, project).Scan(&explicit); err != nil {
		return "", "", err
	}
	if !explicit {
		return "legacy", "valid", nil
	}
	current, healthy, err := migrationCurrentBinding(ctx, tx, project)
	if err != nil {
		return "", "", err
	}
	rows, err := tx.QueryContext(ctx, `SELECT revision,CASE WHEN length(CAST(binding_json AS BLOB))<=4096 THEN binding_json ELSE '' END FROM platform_repository_binding_history WHERE project_id=? ORDER BY revision`, project)
	if err != nil {
		return "", "", err
	}
	var maximum int64
	var initial RepositoryBinding
	matched := false
	for rows.Next() {
		var revision any
		var raw string
		if err = rows.Scan(&revision, &raw); err != nil {
			rows.Close()
			return "", "", err
		}
		r, ok := revision.(int64)
		if !ok {
			healthy = false
			continue
		}
		b, valid := decodeIdentityHistory(raw, r)
		if !valid {
			healthy = false
			continue
		}
		if r > maximum {
			maximum = r
		}
		if r == 1 {
			initial = b
		}
		if r == current.Revision && b == current {
			matched = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", "", err
	}
	healthy = healthy && matched && maximum == current.Revision
	rows, err = tx.QueryContext(ctx, `SELECT actor,request_id,digest,CASE WHEN length(CAST(receipt_json AS BLOB))<=8192 THEN receipt_json ELSE '' END FROM platform_repository_project_receipts WHERE project_id=?`, project)
	if err != nil {
		return "", "", err
	}
	count := 0
	for rows.Next() {
		var actorRaw, requestRaw, digestRaw, rawValue any
		if err = rows.Scan(&actorRaw, &requestRaw, &digestRaw, &rawValue); err != nil {
			rows.Close()
			return "", "", err
		}
		count++
		actor, actorOK := actorRaw.(int64)
		request, requestOK := requestRaw.(string)
		digest, digestOK := digestRaw.(string)
		raw, rawOK := rawValue.(string)
		if !actorOK || !requestOK || !digestOK || !rawOK || !utf8.ValidString(raw) || uniqueScopeJSON([]byte(raw)) != nil {
			healthy = false
			continue
		}
		var receipt RepositoryProjectReceipt
		hash, hashErr := hex.DecodeString(digest)
		fields, shapeErr := scopeObject(json.RawMessage(raw), "project", "repository", "integration_revision", "replayed")
		if actor <= 0 || !repositoryCreationID.MatchString(request) || request != strings.ToLower(request) || hashErr != nil || len(hash) != 32 || shapeErr != nil || json.Unmarshal([]byte(raw), &receipt) != nil || receipt.Project.ID != project || receipt.Project.Name == "" || len(receipt.Project.Name) > 200 || !receipt.Project.Enabled || receipt.Replayed || receipt.IntegrationRevision < 2 || initial.Revision != 1 || receipt.Binding != initial {
			healthy = false
			continue
		}
		if _, err := scopeObject(fields["project"], "id", "name", "enabled"); err != nil {
			healthy = false
			continue
		}
		if b, valid := decodeIdentityHistory(string(fields["repository"]), 1); !valid || b != initial {
			healthy = false
			continue
		}
		expected := receipt.IntegrationRevision - 1
		input := RepositoryProjectInput{RequestID: request, Name: receipt.Project.Name, ExpectedIntegrationRevision: &expected, Provider: initial.Provider, APIOrigin: initial.APIOrigin, RemoteID: initial.RemoteID, FullName: initial.FullName, IntegrationID: initial.IntegrationID}
		normalized, _, inputErr := normalizeRepositoryProjectInput(input)
		body, marshalErr := json.Marshal(normalized)
		sum := sha256.Sum256(body)
		if inputErr != nil || marshalErr != nil || normalized.Name != receipt.Project.Name || hex.EncodeToString(sum[:]) != digest {
			healthy = false
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", "", err
	}
	if count > 1 {
		healthy = false
	}
	if count > 0 {
		var collision bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_runs WHERE project_id=? OR source_project_id=?) OR EXISTS(SELECT 1 FROM platform_run_context_repositories WHERE project_id=?)`, project, project, project).Scan(&collision); err != nil {
			return "", "", err
		}
		// Schema1 never supported auditing native-created projects. Such rows
		// therefore cannot be reassigned to the new project's membership.
		if collision {
			return "", "", errRepositoryHistoryCollision
		}
	}
	if healthy {
		return "internal", "valid", nil
	}
	return "internal", "repair_required", nil
}
