package platform

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestOwnerRoutingRequiresExplicitPermissionsAndTracksRevisions(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	p := OwnerRouting{DefaultOwner: member.ID, Aliases: map[string][]int64{"@org/team": {member.ID}}}
	if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, p); err == nil {
		t.Fatal("unprivileged owner accepted")
	}
	if err := s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveOwnerRouting(ctx, 1, member.ID, 0, p); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("viewer changed routing", err)
	}
	saved, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, p)
	if err != nil || saved.Revision != 1 {
		t.Fatal(saved, err)
	}
	if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, OwnerRouting{}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale update accepted", err)
	}
	read, err := s.OwnerRouting(ctx, 1, member.ID)
	if err != nil || read.DefaultOwner != member.ID || len(read.Aliases["@org/team"]) != 1 {
		t.Fatal(read, err)
	}
	if _, err := s.OwnerRouting(ctx, 2, member.ID); err == nil {
		t.Fatal("cross-project routing disclosed")
	}
	if err := s.SetProjectMember(ctx, 1, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 1, p); err == nil {
		t.Fatal("revoked owner retained by save")
	}
	cleared, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 1, OwnerRouting{})
	if err != nil || cleared.Revision != 2 || cleared.DefaultOwner != 0 || len(cleared.Aliases) != 0 {
		t.Fatal(cleared, err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_owner_routing_history WHERE project_id=1`).Scan(&count); err != nil || count != 2 {
		t.Fatal("history mismatch", count, err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_project_members WHERE user_id=?`, member.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("routing granted permissions", count, err)
	}
}

func TestOwnerRoutingConcurrentConfigurationCannotOverwrite(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, OwnerRouting{DefaultOwner: admin.ID})
			if err == nil {
				wins.Add(1)
			} else if !errors.Is(err, ErrConflict) {
				t.Errorf("unexpected configuration error: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("multiple writers", wins.Load())
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_owner_routing_history`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestOwnerRoutingRejectsInvalidMappingsWithoutHistory(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	for _, p := range []OwnerRouting{
		{DefaultOwner: -1},
		{Aliases: map[string][]int64{" ": {admin.ID}}},
		{Aliases: map[string][]int64{"@team": {0}}},
		{Aliases: map[string][]int64{"@team": {admin.ID, admin.ID}}},
		{Aliases: map[string][]int64{"@team": {}}},
	} {
		if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, p); !errors.Is(err, ErrConflict) {
			t.Fatal("invalid mapping accepted", p, err)
		}
	}
	if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 1<<63-1, OwnerRouting{}); !errors.Is(err, ErrConflict) {
		t.Fatal("revision overflow", err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_owner_routing_history`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected mapping recorded", count, err)
	}
}
