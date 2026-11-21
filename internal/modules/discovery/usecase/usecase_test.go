package usecase

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"waylight/internal/shared/kernel"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

var private = []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16")}

func admin() context.Context {
	return kernel.WithPrincipal(context.Background(), kernel.Principal{AccountID: "a", Role: "admin"})
}

// TestFind uses a fake Docker API and a fake resolver: only candidates
// that answer like Immich are offered, nothing outside the private ranges
// is probed, and the result is cached.
func TestFind(t *testing.T) {
	var mu sync.Mutex
	var probed []string
	c := &clock{now: time.Unix(1_800_000_000, 0)}
	uc := New(Deps{
		Specs: map[string]Spec{"immich": {Service: "Immich", Images: []string{"immich-server"}, Hostnames: []string{"immich.lan"},
			Ports: []int{2283}, ProbePath: "/api/server/ping", ProbeContains: "pong"}},
		Containers: func(context.Context) ([]Container, error) {
			return []Container{
				{Name: "immich_server", Image: "ghcr.io/immich-app/immich-server:v2", Addresses: []netip.Addr{netip.MustParseAddr("172.18.0.5")},
					Ports: []struct{ Private, Public int }{{Private: 2283, Public: 2283}}},
				{Name: "db", Image: "postgres:16", Addresses: []netip.Addr{netip.MustParseAddr("172.18.0.6")}},
			}, nil
		},
		Gateway: func() (netip.Addr, bool) { return netip.MustParseAddr("192.168.178.1"), true },
		Resolve: func(_ context.Context, host string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil // outside: never probed
		},
		Probe: func(_ context.Context, base, path, contains string) bool {
			mu.Lock()
			probed = append(probed, base+path)
			mu.Unlock()
			return base == "http://172.18.0.5:2283" && contains == "pong"
		},
		Allowed: private, Clock: c,
	})

	if _, err := uc.Find(kernel.WithPrincipal(context.Background(), kernel.Principal{AccountID: "m", Role: "member"}), "immich"); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("members: %v", err)
	}
	found, err := uc.Find(admin(), "immich")
	if err != nil || len(found) != 1 || found[0].URL != "http://172.18.0.5:2283" || !strings.Contains(found[0].Hint, "immich_server") {
		t.Fatalf("%+v %v", found, err)
	}
	for _, p := range probed {
		if strings.Contains(p, "8.8.8.8") || strings.Contains(p, "172.18.0.6") {
			t.Fatalf("probed %s", p)
		}
	}
	n := len(probed)
	if _, err := uc.Find(admin(), "immich"); err != nil || len(probed) != n {
		t.Fatal("the result is cached")
	}
	c.now = c.now.Add(cacheTTL)
	_, _ = uc.Find(admin(), "immich")
	if len(probed) == n {
		t.Fatal("searched again after the cache time")
	}
	if _, err := uc.Find(admin(), "nope"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown: %v", err)
	}
}

// TestFindByMDNS uses a fake browser: one query per service type and
// search, answers outside the private ranges are never probed, and only
// confirmed services are offered (#627).
func TestFindByMDNS(t *testing.T) {
	var mu sync.Mutex
	queries := map[string]int{}
	var probed []string
	c := &clock{now: time.Unix(1_800_000_000, 0)}
	uc := New(Deps{
		Specs: map[string]Spec{"smarthome": {Service: "Home Assistant", Ports: []int{8123},
			ProbePath: "/manifest.json", ProbeContains: "Home Assistant",
			ServiceTypes: []string{"_home-assistant._tcp", "_a._tcp", "_b._tcp", "_c._tcp"}}},
		Browse: func(ctx context.Context, serviceType string) ([]Announcement, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("a query must be bounded")
			}
			mu.Lock()
			queries[serviceType]++
			mu.Unlock()
			if serviceType != "_home-assistant._tcp" {
				return nil, nil
			}
			return []Announcement{
				{Instance: "Home", Host: "homeassistant.local", Port: 8124,
					Addrs: []netip.Addr{netip.MustParseAddr("192.168.178.30"), netip.MustParseAddr("8.8.4.4")}},
			}, nil
		},
		Probe: func(_ context.Context, base, _, _ string) bool {
			mu.Lock()
			probed = append(probed, base)
			mu.Unlock()
			return base == "http://192.168.178.30:8124"
		},
		Allowed: private, Clock: c,
	})
	found, err := uc.Find(admin(), "smarthome")
	if err != nil || len(found) != 1 || found[0].URL != "http://192.168.178.30:8124" || found[0].Hint != "mDNS Home" {
		t.Fatalf("%+v %v", found, err)
	}
	if len(probed) != 1 {
		t.Fatalf("only the private address may be probed: %v", probed)
	}
	if queries["_home-assistant._tcp"] != 1 || queries["_c._tcp"] != 0 || len(queries) != maxServiceTypes {
		t.Fatalf("queries: %v", queries)
	}
	if _, err := uc.Find(admin(), "smarthome"); err != nil || queries["_home-assistant._tcp"] != 1 {
		t.Fatalf("a cached search must not query again: %v %v", queries, err)
	}
}
