package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPRContextUpdateRetainsAndRevalidatesSources(t *testing.T) {
	root, snap, f, sources := crossGitFixture(t)
	tools := crossTools(root, snap, sources)
	ctx := context.Background()
	source, _ := tools.file(ctx, readArgs{Path: f.File, Start: 1, End: 3})
	if source.Error != "" {
		t.Fatal(source.Error)
	}
	original := Investigation{ID: "retained", Claim: "original", ObservationIDs: []string{source.ObservationID}, PRContext: validPRContext(source.ObservationID)}
	out, _ := tools.record(ctx, original)
	if out.Error != "" {
		t.Fatal(out.Error)
	}
	update := Investigation{ID: original.ID, Claim: "updated", Status: "investigating", ObservationIDs: original.ObservationIDs}
	out, _ = tools.update(ctx, update)
	if out.Error != "" || tools.ledger[original.ID].PRContext == nil {
		t.Fatal("lost omitted context", out.Error)
	}
	result := tools.investigations()
	result[0].PRContext.Before = "caller mutation"
	result[0].PRContext.EntryPoints[0].ObservationIDs[0] = "foreign"
	if tools.ledger[original.ID].PRContext.Before != original.PRContext.Before || tools.ledger[original.ID].PRContext.EntryPoints[0].ObservationIDs[0] != source.ObservationID {
		t.Fatal("returned context aliases ledger")
	}
	update.ObservationIDs = nil
	out, _ = tools.update(ctx, update)
	if out.Error == "" || !strings.Contains(out.Error, "absent") || tools.ledger[original.ID].Claim != "updated" {
		t.Fatal("inherited source bypassed ownership", out.Error)
	}
	update.ObservationIDs = original.ObservationIDs
	raw, _ := json.Marshal(update)
	update.Claim = strings.Repeat("x", 8000-len(raw)+len(update.Claim)-1)
	raw, _ = json.Marshal(update)
	if len(raw) >= 8000 {
		t.Fatal("test payload itself oversized", len(raw))
	}
	out, _ = tools.update(ctx, update)
	if out.Error == "" || !strings.Contains(out.Error, "8000") || tools.ledger[original.ID].Claim != "updated" {
		t.Fatal("retained context bypassed budget", out.Error)
	}
	update.Claim = "explicit invalid"
	update.PRContext = &PRInvestigationContext{}
	out, _ = tools.update(ctx, update)
	if out.Error == "" || tools.ledger[original.ID].Claim != "updated" {
		t.Fatal("explicit invalid context overwrote ledger")
	}
}
