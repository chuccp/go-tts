package clickguard

import (
	"strings"
	"testing"
	"time"
)

func newTestManager(t *testing.T, cfg Config) (*Manager, time.Time) {
	t.Helper()
	manager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	return manager, now
}

func TestNewChallengeUsesDistinctIcons(t *testing.T) {
	manager, _ := newTestManager(t, Config{Enabled: true, Limit: 10, Window: 10 * time.Minute})
	for attempt := 0; attempt < 100; attempt++ {
		challenge, err := manager.NewChallenge()
		if err != nil {
			t.Fatal(err)
		}
		if challenge.Type != "click" {
			t.Fatalf("type = %q", challenge.Type)
		}
		if !strings.HasPrefix(challenge.Image, "data:image/svg+xml;base64,") {
			t.Fatal("challenge image is not an SVG data URL")
		}
		if len(challenge.Targets) != 3 {
			t.Fatalf("target count = %d, want 3", len(challenge.Targets))
		}
		seen := make(map[string]bool)
		for _, target := range challenge.Targets {
			if !strings.HasPrefix(target, "data:image/svg+xml;base64,") {
				t.Fatal("target preview is not an SVG data URL")
			}
			if seen[target] {
				t.Fatal("target previews contain duplicate icons")
			}
			seen[target] = true
		}
		if challenge.Code == "" || challenge.ExpiresIn != 120 {
			t.Fatalf("unexpected challenge metadata: %+v", challenge)
		}
	}
}

func TestAllowUsesWhicheverIdentityReachesLimit(t *testing.T) {
	manager, now := newTestManager(t, Config{Enabled: true, Limit: 2, Window: 10 * time.Minute})
	if !manager.allowAt("203.0.113.5", "cookie-a", now) {
		t.Fatal("first request rejected")
	}
	if !manager.allowAt("203.0.113.5", "cookie-b", now) {
		t.Fatal("second request rejected")
	}
	if manager.allowAt("203.0.113.5", "cookie-c", now) {
		t.Fatal("same IP was not limited across different cookies")
	}

	if !manager.allowAt("198.51.100.1", "shared-cookie", now) {
		t.Fatal("first cookie request rejected")
	}
	if !manager.allowAt("198.51.100.2", "shared-cookie", now) {
		t.Fatal("second cookie request rejected")
	}
	if manager.allowAt("198.51.100.3", "shared-cookie", now) {
		t.Fatal("same cookie was not limited across different IPs")
	}

	if !manager.allowAt("203.0.113.5", "cookie-c", now.Add(10*time.Minute+time.Second)) {
		t.Fatal("expired requests should leave the rolling window")
	}
}

func TestVerifyCreatesSingleUseTicket(t *testing.T) {
	manager, now := newTestManager(t, Config{Enabled: true, Limit: 2, Window: 10 * time.Minute})
	challenge, err := manager.NewChallenge()
	if err != nil {
		t.Fatal(err)
	}
	var data challengeData
	if err := manager.open(challenge.Code, &data); err != nil {
		t.Fatal(err)
	}
	xs := make([]int, len(data.Points))
	ys := make([]int, len(data.Points))
	for i, item := range data.Points {
		xs[i] = item.X
		ys[i] = item.Y
	}

	result, err := manager.verifyAt(challenge.Code, xs, ys, "client-cookie", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Ticket == "" || result.ExpiresIn != 120 {
		t.Fatalf("unexpected verification result: %+v", result)
	}
	if !manager.ConsumeTicket(result.Ticket, "client-cookie") {
		t.Fatal("valid ticket was rejected")
	}
	if manager.ConsumeTicket(result.Ticket, "client-cookie") {
		t.Fatal("ticket was accepted more than once")
	}
	if _, err := manager.verifyAt(challenge.Code, xs, ys, "client-cookie", now); err == nil {
		t.Fatal("captcha challenge was accepted more than once")
	}
}

func TestVerifyRejectsWrongPointExpiredAndOtherClient(t *testing.T) {
	manager, now := newTestManager(t, Config{Enabled: true, Limit: 2, Window: 10 * time.Minute})
	challenge, err := manager.NewChallenge()
	if err != nil {
		t.Fatal(err)
	}
	var data challengeData
	if err := manager.open(challenge.Code, &data); err != nil {
		t.Fatal(err)
	}
	xs := []int{data.Points[0].X, data.Points[1].X, data.Points[2].X}
	ys := []int{data.Points[0].Y, data.Points[1].Y, data.Points[2].Y}
	xs[1] += tolerance + 1
	if _, err := manager.verifyAt(challenge.Code, xs, ys, "client-cookie", now); err == nil {
		t.Fatal("wrong click coordinates were accepted")
	}

	expiredChallenge, err := manager.NewChallenge()
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.open(expiredChallenge.Code, &data); err != nil {
		t.Fatal(err)
	}
	expiredXs := []int{data.Points[0].X, data.Points[1].X, data.Points[2].X}
	expiredYs := []int{data.Points[0].Y, data.Points[1].Y, data.Points[2].Y}
	if _, err := manager.verifyAt(expiredChallenge.Code, expiredXs, expiredYs, "client-cookie", now.Add(3*time.Minute)); err == nil {
		t.Fatal("expired challenge was accepted")
	}

	validChallenge, err := manager.NewChallenge()
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.open(validChallenge.Code, &data); err != nil {
		t.Fatal(err)
	}
	xs = []int{data.Points[0].X, data.Points[1].X, data.Points[2].X}
	ys = []int{data.Points[0].Y, data.Points[1].Y, data.Points[2].Y}
	result, err := manager.verifyAt(validChallenge.Code, xs, ys, "client-a", now)
	if err != nil {
		t.Fatal(err)
	}
	if manager.ConsumeTicket(result.Ticket, "client-b") {
		t.Fatal("ticket was accepted for a different client cookie")
	}
}

func TestResetClearsBothIdentities(t *testing.T) {
	manager, now := newTestManager(t, Config{Enabled: true, Limit: 1, Window: 10 * time.Minute})
	if !manager.allowAt("203.0.113.9", "cookie-reset", now) {
		t.Fatal("first request rejected")
	}
	if manager.allowAt("203.0.113.9", "cookie-reset", now) {
		t.Fatal("limit was not applied")
	}
	manager.Reset("203.0.113.9", "cookie-reset")
	if !manager.allowAt("203.0.113.9", "cookie-reset", now) {
		t.Fatal("reset did not clear both identities")
	}
}
