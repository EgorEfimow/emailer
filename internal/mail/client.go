package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/egorefimow/emailer/internal/config"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// IMAPClient implements Ingester for IMAP mailboxes.
//
// It uses a single IMAP connection per account (dial once, fetch + flag).
// Call Dial then Login before Fetch, and Close when done.
type IMAPClient struct {
	cli *client.Client
}

// NewIMAPClient returns a new IMAPClient ready for connection.
func NewIMAPClient() *IMAPClient {
	return &IMAPClient{}
}

// Dial connects to the IMAP server using the provided account configuration.
//
// Connection mode is selected based on UseTLS and port:
//   - UseTLS=true:          TLS (imaps) — uses client.DialTLS (default for port 993).
//   - UseTLS=false, port 143: STARTTLS — plain dial then upgrade (standard IMAP).
//   - UseTLS=false, other:   plaintext — no encryption (testing / non-standard setups).
func (c *IMAPClient) Dial(ctx context.Context, account config.IMAPAccount) error {
	addr := fmt.Sprintf("%s:%d", account.Host, account.Port)

	switch {
	case account.UseTLS:
		// TLS (imaps) — encrypted from first byte.
		cli, err := client.DialTLS(addr, nil)
		if err != nil {
			return fmt.Errorf("imap.dial_tls: %w", err)
		}
		c.cli = cli

	case account.Port == 143:
		// STARTTLS — plain dial then upgrade (standard for port 143).
		cli, err := client.Dial(addr)
		if err != nil {
			return fmt.Errorf("imap.dial: %w", err)
		}
		if err := cli.StartTLS(&tls.Config{ServerName: account.Host}); err != nil {
			_ = cli.Logout()
			return fmt.Errorf("imap.starttls: %w", err)
		}
		c.cli = cli

	default:
		// Plaintext — no encryption.
		cli, err := client.Dial(addr)
		if err != nil {
			return fmt.Errorf("imap.dial: %w", err)
		}
		c.cli = cli
	}

	return nil
}

// Login authenticates with the IMAP server using the account credentials.
func (c *IMAPClient) Login(ctx context.Context, account config.IMAPAccount) error {
	if c.cli == nil {
		return fmt.Errorf("imap.login: not connected — call Dial first")
	}

	if err := c.cli.Login(account.Username, account.Password); err != nil {
		return fmt.Errorf("imap.login: %w", err)
	}

	return nil
}

// Close closes the underlying IMAP connection.
func (c *IMAPClient) Close() error {
	if c.cli == nil {
		return nil
	}
	return c.cli.Logout()
}

// Fetch retrieves messages from the account's folders within the fetch window.
func (c *IMAPClient) Fetch(ctx context.Context, account config.IMAPAccount, opts FetchOptions) ([]Message, error) {
	// TODO: implement in steps 7.3–7.7
	panic("not implemented")
}

// ApplyFlags sets IMAP keyword flags on the specified messages.
//
// Flags are written as plain IMAP keywords (no backslash prefix) via
// UID STORE. Errors for individual keyword groups are collected and
// returned as a single error — the method does not abort on partial
// failures.
func (c *IMAPClient) ApplyFlags(ctx context.Context, account config.IMAPAccount, flags []Flag) error {
	// Empty flags is a no-op, even without a connection.
	if len(flags) == 0 {
		return nil
	}
	if c.cli == nil {
		return fmt.Errorf("imap.apply_flags: not connected")
	}

	// Select INBOX in read-write mode (must not be read-only to modify flags).
	// TODO: support per-folder flag application when folder info is added to Flag.
	if _, err := c.selectFolder(ctx, "INBOX", false); err != nil {
		return fmt.Errorf("imap.apply_flags.select: %w", err)
	}

	// Group flags by keyword for efficient IMAP calls.
	byKeyword := make(map[string][]uint32)
	for _, f := range flags {
		byKeyword[f.Keyword] = append(byKeyword[f.Keyword], f.Key.UID)
	}

	var errs []string
	for keyword, uids := range byKeyword {
		seqset := new(imap.SeqSet)
		seqset.AddNum(uids...)

		if err := c.cli.UidStore(seqset, imap.AddFlags, []interface{}{keyword}, nil); err != nil {
			errs = append(errs, fmt.Sprintf("keyword %q on %d UIDs: %v", keyword, len(uids), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("imap.apply_flags: %d/%d groups failed: %s",
			len(errs), len(byKeyword), strings.Join(errs, "; "))
	}

	return nil
}