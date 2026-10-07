package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func notificationSeverity(v string) int {
	switch v {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "info":
		return 1
	}
	return 0
}
func digestWindow(frequency string, at time.Time) (time.Time, time.Time) {
	local := at.In(time.FixedZone("Asia/Shanghai", 8*3600))
	end := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	if frequency == "weekly" {
		days := (int(end.Weekday()) + 6) % 7
		end = end.AddDate(0, 0, -days)
		return end.AddDate(0, 0, -7), end
	}
	return end.AddDate(0, 0, -1), end
}

// Events and routing receipts commit with the outbox rows, so restarts cannot
// mark an event handled without also retaining its delivery record.
func (s *Store) CollectNotifications(ctx context.Context, publicURL string, at time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cursor int64
	if err = tx.QueryRowContext(ctx, `SELECT last_integration FROM platform_notification_collector WHERE id=1`).Scan(&cursor); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations WHERE enabled=1 ORDER BY (id<=?),id LIMIT 200`, cursor)
	if err != nil {
		return err
	}
	integrations := []Integration{}
	for rows.Next() {
		v, _, err := scanIntegration(rows)
		if err != nil {
			rows.Close()
			return err
		}
		switch v.Kind {
		case "email", "feishu", "dingtalk", "wecom", "slack", "teams", "webhook":
			integrations = append(integrations, v)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	processed := 0
	for _, v := range integrations {
		for _, project := range v.ProjectIDs {
			until := at.UTC()
			if v.Frequency != "instant" {
				_, until = digestWindow(v.Frequency, at)
			}
			events, err := tx.QueryContext(ctx, `SELECT e.id,e.run_id,e.kind,e.severity,e.created_at,r.status,r.requested_by FROM platform_notification_events e JOIN platform_runs r ON r.id=e.run_id WHERE r.project_id=? AND julianday(e.created_at)>=julianday(?) AND julianday(e.created_at)<julianday(?) AND NOT EXISTS(SELECT 1 FROM platform_notification_event_routes x WHERE x.integration_id=? AND x.revision=? AND x.event_id=e.id) ORDER BY e.id LIMIT 200`, project, v.UpdatedAt, until.UTC().Format(time.RFC3339Nano), v.ID, v.Revision)
			if err != nil {
				return err
			}
			type event struct {
				id, run, actor        int64
				kind, created, status string
				severity              int
			}
			pending := []event{}
			for events.Next() {
				var e event
				if err = events.Scan(&e.id, &e.run, &e.kind, &e.severity, &e.created, &e.status, &e.actor); err != nil {
					events.Close()
					return err
				}
				pending = append(pending, e)
			}
			err = events.Err()
			events.Close()
			if err != nil {
				return err
			}
			for _, e := range pending {
				included := false
				for _, kind := range v.Events {
					if kind == e.kind {
						included = true
					}
				}
				skip := !included || (e.severity < notificationSeverity(v.MinimumSeverity) && e.status == "succeeded")
				snap, err := snapshotForRun(ctx, tx, e.run)
				if err != nil {
					return err
				}
				if _, err = requireSnapshotRole(ctx, tx, snap, e.actor, "viewer"); err != nil {
					skip = true
				}
				if !skip && len(v.OwnerIDs) > 0 {
					allowed, err := notificationOwnerEventAllowed(ctx, tx, v, e.id, project)
					if err != nil {
						return err
					}
					skip = !allowed
				}
				if !skip && v.Frequency != "instant" {
					created, err := time.Parse(time.RFC3339Nano, e.created)
					if err != nil {
						return err
					}
					_, end := digestWindow(v.Frequency, at)
					if !created.Before(end) {
						continue
					}
				}
				if !skip {
					text := fmt.Sprintf("AIMergeBot · 项目 %d · 审计 #%d · %s · 状态 %s", project, e.run, e.kind, e.status)
					link := ""
					if publicURL != "" {
						link = strings.TrimRight(publicURL, "/") + fmt.Sprintf("/#/runs/%d", e.run)
					}
					summary := NotificationSummary{sourceEventID: e.id, Version: "aimangebot.notification.v1", EventID: fmt.Sprintf("audit-event-v%d-%d", v.Revision, e.id), ProjectID: project, RunID: e.run, Text: text, URL: link}
					if v.Frequency == "instant" {
						if err = insertNotificationOutbox(ctx, tx, v, summary, at); err != nil {
							return err
						}
					} else {
						created, _ := time.Parse(time.RFC3339Nano, e.created)
						start, end := digestWindow(v.Frequency, created.AddDate(0, 0, func() int {
							if v.Frequency == "weekly" {
								return 7
							}
							return 1
						}()))
						key := fmt.Sprintf("digest:v%d:%d:%s:%s", v.Revision, project, v.Frequency, start.Format("2006-01-02"))
						summary.EventID = key
						summary.Text = fmt.Sprintf("AIMergeBot %s汇总 · 项目 %d · %s 至 %s\n", v.Frequency, project, start.Format("2006-01-02"), end.Format("2006-01-02")) + text
						summary.RunID = 0
						summary.URL = strings.TrimRight(publicURL, "/") + "/#/tasks"
						if publicURL == "" {
							summary.URL = ""
						}
						if err = appendDigestOutbox(ctx, tx, v, summary, at, e.run); err != nil {
							return err
						}
					}
				}
				if _, err = tx.ExecContext(ctx, `INSERT INTO platform_notification_event_routes(integration_id,revision,event_id) VALUES(?,?,?)`, v.ID, v.Revision, e.id); err != nil {
					return err
				}
				processed++
				if processed >= 200 {
					if _, err = tx.ExecContext(ctx, `UPDATE platform_notification_collector SET last_integration=? WHERE id=1`, v.ID); err != nil {
						return err
					}
					return tx.Commit()
				}
			}
		}
	}
	return tx.Commit()
}

func insertNotificationOutbox(ctx context.Context, tx *sql.Tx, v Integration, summary NotificationSummary, at time.Time) error {
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	stamp := at.UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_notification_deliveries(integration_id,integration_revision,project_id,run_id,event_key,next_attempt,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, v.ID, v.Revision, summary.ProjectID, summary.RunID, summary.EventID, stamp, string(data), stamp, stamp)
	if err != nil {
		return err
	}
	return recordDeliveryEvent(ctx, tx, v.ID, summary.EventID, summary.sourceEventID)
}
func appendDigestOutbox(ctx context.Context, tx *sql.Tx, v Integration, summary NotificationSummary, at time.Time, sourceRun int64) error {
	var id int64
	var payload, status string
	err := tx.QueryRowContext(ctx, `SELECT id,payload,status FROM platform_notification_deliveries WHERE integration_id=? AND event_key=?`, v.ID, summary.EventID).Scan(&id, &payload, &status)
	if err == sql.ErrNoRows {
		if err = insertNotificationOutbox(ctx, tx, v, summary, at); err != nil {
			return err
		}
		return recordDigestRun(ctx, tx, v.ID, summary.EventID, sourceRun, summary.sourceEventID)
	}
	if err != nil {
		return err
	}
	if status == "pending" {
		var references int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_notification_delivery_runs WHERE delivery_id=?`, id).Scan(&references); err != nil {
			return err
		}
		if references == 0 {
			if _, err = tx.ExecContext(ctx, `UPDATE platform_notification_deliveries SET status='cancelled',error_code='permission_changed',updated_at=? WHERE id=? AND status='pending'`, at.UTC().Format(time.RFC3339Nano), id); err != nil {
				return err
			}
			status = "cancelled"
		}
	}
	if status == "pending" && len(v.OwnerIDs) > 0 {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_notification_delivery_events WHERE delivery_id=?`, id).Scan(&count); err != nil {
			return err
		}
		if count == 0 || count >= 200 {
			status = "full_or_unproven"
		}
	}
	if status != "pending" {
		summary.EventID += fmt.Sprintf(":part:%d", at.UnixNano())
		summary.Text = "补充汇总 · " + summary.Text
		return appendDigestOutbox(ctx, tx, v, summary, at, sourceRun)
	}
	var previous NotificationSummary
	if err = json.Unmarshal([]byte(payload), &previous); err != nil {
		return err
	}
	line := summary.Text[strings.Index(summary.Text, "\n")+1:]
	if len(previous.Text)+len(line)+1 <= 5500 {
		previous.Text += "\n" + line
	} else if !strings.Contains(previous.Text, "更多事件请查看待办中心") {
		previous.Text += "\n更多事件请查看待办中心"
	}
	data, _ := json.Marshal(previous)
	_, err = tx.ExecContext(ctx, `UPDATE platform_notification_deliveries SET payload=?,updated_at=? WHERE id=? AND status='pending'`, string(data), at.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	return recordDigestRun(ctx, tx, v.ID, summary.EventID, sourceRun, summary.sourceEventID)
}

func recordDigestRun(ctx context.Context, tx *sql.Tx, integrationID int64, eventKey string, runID, eventID int64) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_notification_delivery_runs(delivery_id,run_id) SELECT id,? FROM platform_notification_deliveries WHERE integration_id=? AND event_key=?`, runID, integrationID, eventKey)
	if err != nil {
		return err
	}
	return recordDeliveryEvent(ctx, tx, integrationID, eventKey, eventID)
}
