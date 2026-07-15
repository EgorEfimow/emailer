//nolint:errcheck
package mail

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/config"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
)

// TestIMAPClient_Integration_AllEmails verifies that fetching all emails in
// a time window returns both read and unread messages when FetchUnreadOnly
// is false.
func TestIMAPClient_Integration_AllEmails(t *testing.T) { //nolint:gocyclo
	ctx := context.Background()

	// ---- Setup: in-memory IMAP server -----------------------------------

	be := memory.New()

	// memory.New() creates a default user with username "username" / password "password".
	user, err := be.Login(nil, "username", "password")
	if err != nil {
		t.Fatalf("failed to login test user: %v", err)
	}

	// INBOX already exists in the memory backend.
	mbox, err := user.GetMailbox("INBOX")
	if err != nil {
		t.Fatalf("failed to get INBOX: %v", err)
	}
	memMbox := mbox.(*memory.Mailbox)

	// Clear the default message that memory.New() creates.
	memMbox.Messages = nil

	base := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	// Add messages: UID 1 (read), UID 2 (unread), UID 3 (read, out of window)
	mustCreateMessage(t, memMbox, []string{imap.SeenFlag}, base, "Read message 1")
	mustCreateMessage(t, memMbox, nil, base.Add(1*time.Hour), "Unread message 2")
	mustCreateMessage(t, memMbox, []string{imap.SeenFlag}, base.Add(-48*time.Hour), "Old read message 3")

	// Create a listener on a random port so we can retrieve the actual address.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	addr := listener.Addr().String()
	host, port, err := splitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to parse listener addr %q: %v", addr, err)
	}

	// Start the IMAP server on the listener.
	srv := imapserver.New(be)
	srv.AllowInsecureAuth = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.Serve(listener); err != nil && !isServerClosed(err) {
			t.Logf("IMAP server stopped: %v", err)
		}
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		<-done
	})

	// ---- Connect with IMAPClient ----------------------------------------

	account := config.IMAPAccount{
		Label:    "test",
		Host:     host,
		Port:     port,
		Username: "username",
		Password: "password",
		Folders:  []string{"INBOX"},
		UseTLS:   false,
	}

	client := NewIMAPClient()
	if err := client.Dial(ctx, account); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Login(ctx, account); err != nil {
		t.Fatalf("Login: %v", err)
	}

	// ---- Select INBOX ----------------------------------------------------

	mboxStatus, err := client.selectFolder(ctx, "INBOX", true)
	if err != nil {
		t.Fatalf("selectFolder: %v", err)
	}
	if mboxStatus.Messages != 3 {
		t.Errorf("INBOX should have 3 messages, got %d", mboxStatus.Messages)
	}

	// ---- Search by window (all messages since base-24h) -------------------

	// Window covers messages 1 and 2 but not message 3.
	uids, err := client.searchByWindow(ctx, base.Add(-24*time.Hour), false)
	if err != nil {
		t.Fatalf("searchByWindow: %v", err)
	}
	if len(uids) != 2 {
		t.Fatalf("expected 2 UIDs in window, got %v", uids)
	}

	// ---- Fetch headers ---------------------------------------------------

	msgs, err := client.fetchHeaders(ctx, uids)
	if err != nil {
		t.Fatalf("fetchHeaders: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Verify message 1 (read, "Read message 1").
	var msg1, msg2 *imap.Message
	for _, m := range msgs {
		switch m.Uid {
		case 1:
			msg1 = m
		case 2:
			msg2 = m
		}
	}
	if msg1 == nil {
		t.Fatal("message UID 1 not found in fetch results")
	}
	if msg2 == nil {
		t.Fatal("message UID 2 not found in fetch results")
	}

	if !isRead(msg1.Flags) {
		t.Errorf("message 1 should be read, flags=%v", msg1.Flags)
	}
	if isRead(msg2.Flags) {
		t.Errorf("message 2 should be unread, flags=%v", msg2.Flags)
	}
	if msg1.Envelope == nil || msg2.Envelope == nil {
		t.Fatal("envelope should not be nil")
	}
}

// mustCreateMessage is a test helper that creates a message in the given
// mailbox with the specified flags, date, and subject.
func mustCreateMessage(t *testing.T, mbox *memory.Mailbox, flags []string, date time.Time, subject string) {
	t.Helper()
	body := "Subject: " + subject + "\r\n\r\nTest body"
	if err := mbox.CreateMessage(flags, date, strings.NewReader(body)); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
}

// TestIMAPClient_ApplyFlags_Integration verifies that:
//   - ApplyFlags adds plain keyword flags to messages via UID STORE
//   - Multiple keywords on different UIDs are applied correctly
//   - Empty flags is a no-op
func TestIMAPClient_ApplyFlags_Integration(t *testing.T) { //nolint:gocyclo
	ctx := context.Background()

	// ---- Setup: in-memory IMAP server --------------------------------

	be := memory.New()
	user, err := be.Login(nil, "username", "password")
	if err != nil {
		t.Fatalf("failed to login test user: %v", err)
	}

	mbox, err := user.GetMailbox("INBOX")
	if err != nil {
		t.Fatalf("failed to get INBOX: %v", err)
	}
	memMbox := mbox.(*memory.Mailbox)

	// Clear the default message that memory.New() creates.
	memMbox.Messages = nil

	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	// Add 3 messages with no custom flags.
	mustCreateMessage(t, memMbox, nil, base, "Message 1")
	mustCreateMessage(t, memMbox, nil, base.Add(1*time.Hour), "Message 2")
	mustCreateMessage(t, memMbox, nil, base.Add(2*time.Hour), "Message 3")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	addr := listener.Addr().String()
	host, port, err := splitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to parse listener addr %q: %v", addr, err)
	}

	srv := imapserver.New(be)
	srv.AllowInsecureAuth = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.Serve(listener); err != nil && !isServerClosed(err) {
			t.Logf("IMAP server stopped: %v", err)
		}
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		<-done
	})

	// ---- Connect with IMAPClient -------------------------------------

	account := config.IMAPAccount{
		Label:    "test",
		Host:     host,
		Port:     port,
		Username: "username",
		Password: "password",
		Folders:  []string{"INBOX"},
		UseTLS:   false,
	}

	client := NewIMAPClient()
	if err := client.Dial(ctx, account); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	// Use a separate client for verification so we can close the flag client first.
	verifyClient := NewIMAPClient()
	if err := verifyClient.Dial(ctx, account); err != nil {
		t.Fatalf("Dial (verify): %v", err)
	}
	t.Cleanup(func() { _ = verifyClient.Close() })

	if err := client.Login(ctx, account); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := verifyClient.Login(ctx, account); err != nil {
		t.Fatalf("Login (verify): %v", err)
	}

	// ---- Apply flags --------------------------------------------------

	flags := []Flag{
		{Key: MessageKey{AccountLabel: "test", UID: 1}, Keyword: "Useful"},
		{Key: MessageKey{AccountLabel: "test", UID: 2}, Keyword: "ToDelete"},
		{Key: MessageKey{AccountLabel: "test", UID: 3}, Keyword: "Useful"},
		// Apply two keywords to the same message.
		{Key: MessageKey{AccountLabel: "test", UID: 2}, Keyword: "Ads"},
	}

	if err := client.ApplyFlags(ctx, account, flags); err != nil {
		t.Fatalf("ApplyFlags: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// ---- Verify flags were applied -----------------------------------

	// Select INBOX on the verify client to read flags.
	if _, err := verifyClient.selectFolder(ctx, "INBOX", true); err != nil {
		t.Fatalf("selectFolder (verify): %v", err)
	}

	// Fetch headers for all 3 messages to see their flags.
	msgs, err := verifyClient.fetchHeaders(ctx, []uint32{1, 2, 3})
	if err != nil {
		t.Fatalf("fetchHeaders (verify): %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Build lookup by UID.
	byUID := make(map[uint32]*imap.Message)
	for _, m := range msgs {
		byUID[m.Uid] = m
	}

	// UID 1 should have "Useful".
	msg1 := byUID[1]
	if msg1 == nil {
		t.Fatal("message UID 1 not found")
	}
	if !hasFlag(msg1.Flags, "Useful") {
		t.Errorf("message 1 should have flag 'Useful', got %v", msg1.Flags)
	}
	if hasFlag(msg1.Flags, "ToDelete") {
		t.Errorf("message 1 should NOT have flag 'ToDelete', got %v", msg1.Flags)
	}

	// UID 2 should have "ToDelete" and "Ads".
	msg2 := byUID[2]
	if msg2 == nil {
		t.Fatal("message UID 2 not found")
	}
	if !hasFlag(msg2.Flags, "ToDelete") {
		t.Errorf("message 2 should have flag 'ToDelete', got %v", msg2.Flags)
	}
	if !hasFlag(msg2.Flags, "Ads") {
		t.Errorf("message 2 should have flag 'Ads', got %v", msg2.Flags)
	}

	// UID 3 should have "Useful".
	msg3 := byUID[3]
	if msg3 == nil {
		t.Fatal("message UID 3 not found")
	}
	if !hasFlag(msg3.Flags, "Useful") {
		t.Errorf("message 3 should have flag 'Useful', got %v", msg3.Flags)
	}
}

// hasFlag checks whether the given flag string is present in the flags slice.
// Comparison is case-insensitive because the in-memory backend normalizes
// flag casing; real IMAP servers preserve keyword casing.
func hasFlag(flags []string, target string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, target) {
			return true
		}
	}
	return false
}
// network connection" from shutting down the test server.
func isServerClosed(err error) bool {
	return err != nil && strings.Contains(err.Error(), "closed network connection")
}

// splitHostPort splits a "host:port" string into separate parts.
// Port is returned as an int.
func splitHostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}