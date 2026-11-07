//go:build livepanel

// Live integration test against a real 3X-UI panel (Stage G).
//
// Run with:
//
//	$env:VEYLIX_PANEL_TEST_URL  = "https://host:2053/<web-base-path>"
//	$env:VEYLIX_PANEL_TEST_TOKEN = "<api token>"
//	go test -tags livepanel -run TestLive -v ./internal/panel/
//
// The build tag keeps it out of the normal unit run (it needs a network and a
// real credential). It is READ-ONLY: no client is created, changed or deleted.
package panel

import (
	"context"
	"os"
	"testing"
	"time"
)

func liveClient(t *testing.T) *Client {
	t.Helper()
	rawURL := os.Getenv("VEYLIX_PANEL_TEST_URL")
	token := os.Getenv("VEYLIX_PANEL_TEST_TOKEN")
	if rawURL == "" || token == "" {
		t.Skip("VEYLIX_PANEL_TEST_URL / VEYLIX_PANEL_TEST_TOKEN not set")
	}

	target, err := ParseTarget(rawURL, true, nil)
	if err != nil {
		// An IP literal needs no resolver; a hostname does.
		target, err = ParseTarget(rawURL, true, defaultResolver)
		if err != nil {
			t.Fatalf("ParseTarget(%q): %v", rawURL, err)
		}
	}

	c, err := New(Options{Target: target, Token: token, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestLiveHealth(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	res := c.Health(ctx)
	if res.Status != StatusActive {
		t.Fatalf("health status = %s (err=%v)", res.Status, res.Err)
	}
	if res.PanelVersion == "" {
		t.Error("panel version not reported")
	}
	t.Logf("LIVE OK: panel=%s xray=%s state=%s cpu=%.2f mem=%d/%d latency=%dms",
		res.PanelVersion, res.XrayVersion, res.XrayState, res.CPUPct, res.MemCurrent, res.MemTotal, res.LatencyMS)
}

func TestLiveInbounds(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	inbounds, err := c.Inbounds(ctx)
	if err != nil {
		t.Fatalf("Inbounds: %v", err)
	}
	if len(inbounds) == 0 {
		t.Fatal("panel reported zero inbounds — provisioning would have no target")
	}
	for _, ib := range inbounds {
		t.Logf("LIVE inbound: id=%d remark=%q protocol=%s port=%d enable=%v",
			ib.ID, ib.Remark, ib.Protocol, ib.Port, ib.Enable)
	}
}

func TestLiveClientList(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	page, err := c.ListClients(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	t.Logf("LIVE clients: total=%d returned=%d active=%d online=%d",
		page.Total, len(page.Items), page.Summary.Active, page.Summary.OnlineCount)

	// A fresh read of an existing client must resolve to "present".
	if len(page.Items) > 0 {
		email := page.Items[0].Email
		exists, err := c.ClientExists(ctx, email)
		if err != nil {
			t.Fatalf("ClientExists(%q): %v", email, err)
		}
		if !exists {
			t.Errorf("client %q listed but not found by fresh read", email)
		}
		t.Logf("LIVE fresh-read OK for %q", email)
	}
}
