package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var ErrRepositoryProjectInput = errors.New("invalid repository project configuration")
var repositoryCreationID = regexp.MustCompile(`\A[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\z`)

type RepositoryProjectInput struct {
	RequestID                   string `json:"request_id"`
	Name                        string `json:"name"`
	ExpectedIntegrationRevision *int64 `json:"expected_integration_revision"`
	Provider                    string `json:"provider"`
	APIOrigin                   string `json:"api_origin"`
	RemoteID                    int64  `json:"remote_id"`
	FullName                    string `json:"full_name"`
	IntegrationID               int64  `json:"integration_id"`
}

// RepositoryProjectReceipt is the creation record, not live project configuration.
type RepositoryProjectReceipt struct {
	Project             Project           `json:"project"`
	Binding             RepositoryBinding `json:"repository"`
	IntegrationRevision int64             `json:"integration_revision"`
	Replayed            bool              `json:"replayed"`
}

func migrateRepositoryProjectReceipts(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_repository_project_receipts(actor INTEGER NOT NULL REFERENCES platform_users(id),request_id TEXT NOT NULL,digest TEXT NOT NULL,project_id INTEGER NOT NULL REFERENCES platform_projects(id),receipt_json TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(actor,request_id))`)
	return err
}

func normalizeRepositoryProjectInput(input RepositoryProjectInput) (RepositoryProjectInput, RepositoryBinding, error) {
	if !repositoryCreationID.MatchString(input.RequestID) || len(input.Name) > 200 || strings.TrimSpace(input.Name) == "" || input.ExpectedIntegrationRevision == nil || *input.ExpectedIntegrationRevision <= 0 || *input.ExpectedIntegrationRevision == 1<<63-1 {
		return input, RepositoryBinding{}, ErrRepositoryProjectInput
	}
	binding, err := validateRepositoryBinding(RepositoryBinding{Provider: input.Provider, APIOrigin: input.APIOrigin, RemoteID: input.RemoteID, FullName: input.FullName, IntegrationID: input.IntegrationID})
	if err != nil {
		return input, RepositoryBinding{}, ErrRepositoryProjectInput
	}
	input.RequestID = strings.ToLower(input.RequestID)
	input.Name = strings.TrimSpace(input.Name)
	input.APIOrigin = binding.APIOrigin
	return input, binding, nil
}

func (s *Store) CreateRepositoryProject(ctx context.Context, actor int64, input RepositoryProjectInput) (RepositoryProjectReceipt, error) {
	input, binding, err := normalizeRepositoryProjectInput(input)
	if err != nil {
		return RepositoryProjectReceipt{}, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return RepositoryProjectReceipt{}, err
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RepositoryProjectReceipt{}, err
	}
	defer tx.Rollback()
	if err = requireIntegrationAdmin(ctx, tx, actor); err != nil {
		return RepositoryProjectReceipt{}, err
	}
	var savedDigest, savedRaw string
	var savedProject int
	err = tx.QueryRowContext(ctx, `SELECT digest,project_id,CASE WHEN length(CAST(receipt_json AS BLOB))<=8192 THEN receipt_json ELSE '' END FROM platform_repository_project_receipts WHERE actor=? AND request_id=?`, actor, input.RequestID).Scan(&savedDigest, &savedProject, &savedRaw)
	if err == nil {
		var receipt RepositoryProjectReceipt
		expectedBinding := binding
		expectedBinding.Revision = 1
		if savedDigest != digest || json.Unmarshal([]byte(savedRaw), &receipt) != nil || receipt.Project.ID <= 0 || receipt.Project.ID != savedProject || receipt.Project.Name != input.Name || !receipt.Project.Enabled || receipt.Binding != expectedBinding || receipt.IntegrationRevision != *input.ExpectedIntegrationRevision+1 || receipt.Replayed {
			return RepositoryProjectReceipt{}, ErrConflict
		}
		if err = tx.Commit(); err != nil {
			return RepositoryProjectReceipt{}, err
		}
		receipt.Replayed = true
		return receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RepositoryProjectReceipt{}, err
	}
	integration, credentials, err := scanIntegration(tx.QueryRowContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations WHERE id=?`, binding.IntegrationID))
	if err != nil {
		return RepositoryProjectReceipt{}, err
	}
	origin, originErr := canonicalRepositoryAPIOrigin(binding.Provider, credentials.Endpoint)
	if integration.Revision != *input.ExpectedIntegrationRevision || integration.Kind != binding.Provider || !integration.Enabled || strings.TrimSpace(credentials.Token) == "" || originErr != nil || origin != binding.APIOrigin || len(integration.ProjectIDs) >= 100 {
		return RepositoryProjectReceipt{}, ErrConflict
	}
	var highest int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM platform_projects`).Scan(&highest); err != nil {
		return RepositoryProjectReceipt{}, err
	}
	if highest < 0 || highest == 1<<63-1 || int64(int(highest+1)) != highest+1 {
		return RepositoryProjectReceipt{}, ErrConflict
	}
	project := Project{ID: int(highest + 1), Name: input.Name, Enabled: true}
	if err = saveProjectTx(ctx, tx, project); err != nil {
		return RepositoryProjectReceipt{}, err
	}
	integration.ProjectIDs = append(integration.ProjectIDs, project.ID)
	updated, err := saveIntegrationTx(ctx, tx, integration.ID, actor, IntegrationInput{Integration: integration, ExpectedRevision: input.ExpectedIntegrationRevision})
	if err != nil {
		return RepositoryProjectReceipt{}, err
	}
	savedBinding, err := saveRepositoryBindingTx(ctx, tx, project.ID, actor, 0, binding)
	if err != nil {
		return RepositoryProjectReceipt{}, err
	}
	receipt := RepositoryProjectReceipt{Project: project, Binding: savedBinding, IntegrationRevision: updated.Revision}
	raw, err = json.Marshal(receipt)
	if err != nil || len(raw) > 8192 {
		return RepositoryProjectReceipt{}, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_repository_project_receipts(actor,request_id,digest,project_id,receipt_json,created_at) VALUES(?,?,?,?,?,?)`, actor, input.RequestID, digest, project.ID, string(raw), now()); err != nil {
		return RepositoryProjectReceipt{}, err
	}
	if err = tx.Commit(); err != nil {
		return RepositoryProjectReceipt{}, err
	}
	return receipt, nil
}
