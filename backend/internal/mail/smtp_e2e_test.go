package mail

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a minimal SMTP server: enough to prove the client speaks the
// protocol correctly (EHLO, AUTH, MAIL, RCPT, DATA, QUIT) and that what arrives
// is the message we think we sent.
//
// It exists because the other tests in this file never open a socket: they prove
// the config decisions and the MIME bytes, not that a send actually completes.
// A relay that rejects our handshake would look perfectly healthy there.
type fakeSMTP struct {
	ln   net.Listener
	mu   sync.Mutex
	rcpt []string
	from string
	data string
	auth string
	// reject, when set, answers the command it names with "550" once.
	reject string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeSMTP{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f
}

func (f *fakeSMTP) port() int {
	return f.ln.Addr().(*net.TCPAddr).Port
}

func (f *fakeSMTP) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeSMTP) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	say := func(s string) {
		_, _ = w.WriteString(s + "\r\n")
		_ = w.Flush()
	}
	say("220 fake.smtp.test ESMTP")

	inData := false
	var body strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				f.mu.Lock()
				f.data = body.String()
				f.mu.Unlock()
				say("250 2.0.0 Ok: queued")
				continue
			}
			// SMTP dot-unstuffing: a leading ".." means a literal ".".
			body.WriteString(strings.TrimPrefix(line, ".") + "\n")
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			// No STARTTLS advertised: this fake is plaintext, and the client must
			// simply carry on rather than demand TLS.
			say("250-fake.smtp.test")
			say("250-AUTH PLAIN")
			say("250 8BITMIME")
		case strings.HasPrefix(upper, "AUTH"):
			f.mu.Lock()
			f.auth = line
			f.mu.Unlock()
			say("235 2.7.0 Authentication successful")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			f.mu.Lock()
			f.from = strings.TrimSpace(line[len("MAIL FROM:"):])
			f.mu.Unlock()
			if f.reject == "MAIL" {
				say("550 5.1.0 sender rejected")
				f.reject = ""
				continue
			}
			say("250 2.1.0 Ok")
		case strings.HasPrefix(upper, "RCPT TO:"):
			f.mu.Lock()
			f.rcpt = append(f.rcpt, strings.TrimSpace(line[len("RCPT TO:"):]))
			f.mu.Unlock()
			if f.reject == "RCPT" {
				say("550 5.1.1 no such user")
				f.reject = ""
				continue
			}
			say("250 2.1.5 Ok")
		case strings.HasPrefix(upper, "DATA"):
			inData = true
			say("354 End data with <CR><LF>.<CR><LF>")
		case upper == "QUIT":
			say("221 2.0.0 Bye")
			return
		default:
			say("250 2.0.0 Ok")
		}
	}
}

// A complete send must actually traverse SMTP and arrive intact.
func TestSendDeliversThroughARealSMTPConversation(t *testing.T) {
	f := newFakeSMTP(t)
	s := New(Config{
		Host:     "127.0.0.1",
		Port:     f.port(),
		Username: "smtp-user",
		Password: "smtp-pass",
		From:     "Veylix <no-reply@veylix.test>",
		Timeout:  5 * time.Second,
	})
	if !s.Configured() {
		t.Fatal("sender with host and From must be configured")
	}

	err := s.Send(context.Background(), "owner@example.com", "Your Veylix code is 482913",
		"Your code is 482913", `<p>Your code is <strong>482913</strong></p>`)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.rcpt) != 1 || !strings.Contains(f.rcpt[0], "owner@example.com") {
		t.Fatalf("RCPT = %v, want the recipient", f.rcpt)
	}
	if !strings.Contains(f.from, "no-reply@veylix.test") {
		t.Fatalf("MAIL FROM = %q", f.from)
	}
	if !strings.HasPrefix(f.auth, "AUTH") {
		t.Fatal("the client never authenticated")
	}
	for _, want := range []string{"Your Veylix code is 482913", "482913", "multipart/alternative"} {
		if !strings.Contains(f.data, want) {
			t.Errorf("delivered message is missing %q", want)
		}
	}
}

// A relay that refuses the recipient must surface as an error, not a silent
// success: the caller tells the user a code was sent.
func TestSendReportsARefusedRecipient(t *testing.T) {
	f := newFakeSMTP(t)
	f.reject = "RCPT"
	s := New(Config{
		Host: "127.0.0.1", Port: f.port(), From: "no-reply@veylix.test", Timeout: 5 * time.Second,
	})
	err := s.Send(context.Background(), "gone@example.com", "s", "t", "")
	if err == nil {
		t.Fatal("a 550 on RCPT must be an error")
	}
	if !strings.Contains(err.Error(), "RCPT") {
		t.Fatalf("error = %v, want it to name the RCPT step", err)
	}
}

// A closed port must fail with a dial error, not hang until the suite times out.
func TestSendFailsFastWhenTheRelayIsDown(t *testing.T) {
	// Bind then close, so the port is almost certainly free.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	s := New(Config{
		Host: "127.0.0.1", Port: port, From: "no-reply@veylix.test", Timeout: 2 * time.Second,
	})
	start := time.Now()
	err = s.Send(context.Background(), "a@example.com", "s", "t", "")
	if err == nil {
		t.Fatal("sending to a closed port must fail")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("dial took %s, want a fast failure", elapsed)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(port)) {
		t.Fatalf("error = %v, want it to name the port", err)
	}
}
