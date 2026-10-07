package platform

import "database/sql"

func migrateNotificationEvents(tx *sql.Tx) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS platform_notification_events(id INTEGER PRIMARY KEY,run_id INTEGER NOT NULL REFERENCES platform_runs(id),kind TEXT NOT NULL,event_key TEXT NOT NULL UNIQUE,severity INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_notification_event_findings(event_id INTEGER PRIMARY KEY REFERENCES platform_notification_events(id),finding_id TEXT NOT NULL,disposition_revision INTEGER NOT NULL,owner INTEGER NOT NULL,head_sha TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_notification_delivery_runs(delivery_id INTEGER NOT NULL REFERENCES platform_notification_deliveries(id),run_id INTEGER NOT NULL REFERENCES platform_runs(id),PRIMARY KEY(delivery_id,run_id))`,
		`CREATE INDEX IF NOT EXISTS platform_notification_event_window ON platform_notification_events(created_at,id)`,
		`CREATE TABLE IF NOT EXISTS platform_notification_collector(id INTEGER PRIMARY KEY CHECK(id=1),last_integration INTEGER NOT NULL DEFAULT 0)`,
		`INSERT OR IGNORE INTO platform_notification_collector(id) VALUES(1)`,
		`CREATE TABLE IF NOT EXISTS platform_notification_event_routes(integration_id INTEGER NOT NULL,revision INTEGER NOT NULL,event_id INTEGER NOT NULL,PRIMARY KEY(integration_id,revision,event_id))`,
		`CREATE TRIGGER IF NOT EXISTS platform_notification_run_event AFTER UPDATE OF status ON platform_runs WHEN NEW.status!=OLD.status AND NEW.status IN ('succeeded','incomplete','failed') BEGIN INSERT OR IGNORE INTO platform_notification_events(run_id,kind,event_key,severity,created_at) VALUES(NEW.id,CASE WHEN NEW.status='failed' THEN 'run.failed' ELSE 'run.completed' END,'run:'||NEW.id||':'||NEW.status,COALESCE((SELECT MAX(CASE json_extract(value,'$.severity') WHEN 'critical' THEN 5 WHEN 'high' THEN 4 WHEN 'medium' THEN 3 WHEN 'low' THEN 2 ELSE 1 END) FROM json_each(NEW.result_json,'$.findings')),0),COALESCE(NEW.finished_at,strftime('%Y-%m-%dT%H:%M:%fZ','now'))); END`,
		`DROP TRIGGER IF EXISTS platform_notification_review_insert`,
		`CREATE TRIGGER IF NOT EXISTS platform_notification_review_insert AFTER INSERT ON platform_reviews BEGIN INSERT OR IGNORE INTO platform_notification_events(run_id,kind,event_key,severity,created_at) VALUES(NEW.run_id,'finding.reviewed','review:'||NEW.run_id||':'||NEW.finding_id||':'||NEW.revision,COALESCE((SELECT CASE severity WHEN 'critical' THEN 5 WHEN 'high' THEN 4 WHEN 'medium' THEN 3 WHEN 'low' THEN 2 ELSE 1 END FROM platform_finding_index WHERE run_id=NEW.run_id AND finding_id=NEW.finding_id LIMIT 1),0),NEW.updated_at); ` + reviewNotificationFindingEvidenceSQL + ` END`,
		`DROP TRIGGER IF EXISTS platform_notification_review_update`,
		`CREATE TRIGGER IF NOT EXISTS platform_notification_review_update AFTER UPDATE OF revision ON platform_reviews WHEN NEW.revision!=OLD.revision BEGIN INSERT OR IGNORE INTO platform_notification_events(run_id,kind,event_key,severity,created_at) VALUES(NEW.run_id,'finding.reviewed','review:'||NEW.run_id||':'||NEW.finding_id||':'||NEW.revision,COALESCE((SELECT CASE severity WHEN 'critical' THEN 5 WHEN 'high' THEN 4 WHEN 'medium' THEN 3 WHEN 'low' THEN 2 ELSE 1 END FROM platform_finding_index WHERE run_id=NEW.run_id AND finding_id=NEW.finding_id LIMIT 1),0),NEW.updated_at); ` + reviewNotificationFindingEvidenceSQL + ` END`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Review identifiers are SQL values, never parsed from the opaque event key.
const reviewNotificationFindingEvidenceSQL = `INSERT OR IGNORE INTO platform_notification_event_findings(event_id,finding_id,disposition_revision,owner,head_sha)
SELECT e.id,NEW.finding_id,COALESCE(d.revision,0),COALESCE(d.owner,0),r.head_sha
FROM platform_notification_events e JOIN platform_runs r ON r.id=e.run_id
LEFT JOIN platform_finding_dispositions d ON d.run_id=e.run_id AND d.finding_id=NEW.finding_id
WHERE e.event_key='review:'||NEW.run_id||':'||NEW.finding_id||':'||NEW.revision;`
