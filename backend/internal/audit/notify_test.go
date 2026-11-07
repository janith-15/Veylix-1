package audit

import (
	"strings"
	"testing"
)

// I-38 regression: a refused SIGN-IN must never reach the customer inbox.
//
// The defect (2026-11-07): every failed login was audited with ActorID = the
// account being probed, notify() then pushed it to that account, and describe()
// titled every refusal "Action not applied" with the body "user 15 ·
// result=DENIED". One client's inbox held 16 identical, meaningless rows.
//
// The second half of the defect is a disclosure: the row names the SUBJECT, so
// anybody guessing passwords can watch the victim's inbox fill and confirm the
// account exists without ever signing in.
func TestSignInFailureIsNotNotified(t *testing.T) {
	if !isSignInFailure(ActionLoginFailed) {
		t.Fatal("ActionLoginFailed must be treated as a sign-in failure; " +
			"otherwise a refused login is pushed into the victim's inbox again (I-38)")
	}

	// The guard must not silence real business refusals: those are the ones the
	// operator actually needs to hear about.
	for _, a := range []string{
		ActionProvision, ActionRevoke, ActionSuspend, ActionResume,
		ActionPaymentConfirm, ActionPaymentReject, ActionQuotaAdjust,
		ActionPasswordChangeFailed,
	} {
		if isSignInFailure(a) {
			t.Errorf("%s is a business refusal, not a sign-in failure; "+
				"silencing it would hide work that needs attention", a)
		}
	}
}

// A refused business action still gets a human sentence, never one generic
// label shared by every failure.
func TestRefusalTitlesAreSpecific(t *testing.T) {
	cases := map[string]string{
		ActionProvision:            "VPN could not be created",
		ActionRevoke:               "VPN could not be revoked",
		ActionSuspend:              "VPN could not be suspended",
		ActionResume:               "VPN could not be resumed",
		ActionPaymentConfirm:       "Payment decision was not applied",
		ActionPaymentReject:        "Payment decision was not applied",
		ActionQuotaAdjust:          "Quota change was not applied",
		ActionPasswordChangeFailed: "Password change was refused",
	}
	for action, want := range cases {
		_, got := describe(Entry{Action: action, Result: "DENIED"})
		if got != want {
			t.Errorf("describe(%s, DENIED) title = %q, want %q", action, got, want)
		}
	}

	// "Action not applied" was the single failure title before the fix; it must
	// not come back as the answer for a known action.
	for action := range cases {
		if _, got := describe(Entry{Action: action, Result: "DENIED"}); got == "Action not applied" {
			t.Errorf("describe(%s) fell back to the old generic title", action)
		}
	}
}

// The body is read by a person: no numeric self-identifier, no audit vocabulary.
func TestSummariseNeverEchoesTheReadersOwnID(t *testing.T) {
	got := summarise(Entry{
		TargetType: "user",
		TargetID:   "15",
		Result:     "DENIED",
	})
	if strings.Contains(got, "user 15") {
		t.Errorf("summarise() named the reader back at themselves: %q", got)
	}
	if strings.Contains(got, "result=") {
		t.Errorf("summarise() leaked audit vocabulary into an inbox body: %q", got)
	}

	// A panel client is the one case where the identifier IS the useful subject.
	if body := summarise(Entry{TargetType: "panel_client", TargetID: "vyx-16"}); body != "vyx-16" {
		t.Errorf("summarise(panel_client) = %q, want the panel e-mail", body)
	}
}
