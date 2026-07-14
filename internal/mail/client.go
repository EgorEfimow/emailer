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

	if c.cli == nil {
		return nil, fmt.Errorf("imap.fetch: not connected")
	}

	folders := opts.Folders
	if len(folders) == 0 {
		folders = account.Folders
	}
	if len(folders) == 0 {
		folders = []string{"INBOX"}
	}

	messages := make([]Message, 0)
	var errs []string
	for _, folder := range folders {
		select {
		case <-ctx.Done():
			return messages, fmt.Errorf("imap.fetch: %w", ctx.Err())
		default:
		}

		folderMessages, err := c.fetchFolder(ctx, account, folder, opts)
		if err != nil {
			errs = append(errs, fmt.Sprintf("folder %q: %v", folder, err))
			continue
		}
		messages = append(messages, folderMessages...)
	}

	if len(errs) > 0 {
		return messages, fmt.Errorf("imap.fetch: %d/%d folders failed: %s", len(errs), len(folders), strings.Join(errs, "; "))
	}

	return messages, nil
}

func (c *IMAPClient) fetchFolder(ctx context.Context, account config.IMAPAccount, folder string, opts FetchOptions) ([]Message, error) {
	if _, err := c.selectFolder(ctx, folder, true); err != nil {
		return nil, err
	}

	uids, err := c.searchByWindow(ctx, opts.Since, opts.FetchUnreadOnly)
	if err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return nil, nil
	}

	headers, err := c.fetchHeaders(ctx, uids)
	if err != nil {
		return nil, err
	}

	bodies, bodyErr := c.fetchBody(ctx, uids)

	messages := make([]Message, 0, len(headers))
	for _, header := range headers {
		if header == nil {
			continue
		}

		msg := Message{
			AccountLabel: account.Label,
			UID:          header.Uid,
			Folder:       folder,
			Date:         header.InternalDate,
			IsRead:       isRead(header.Flags),
		}
		if header.Envelope != nil {
			msg.Subject = header.Envelope.Subject
			msg.From = formatAddressList(header.Envelope.From)
			msg.To = formatAddressList(header.Envelope.To)
			if !header.Envelope.Date.IsZero() {
				msg.Date = header.Envelope.Date
			}
		}
		if body, ok := bodies[header.Uid]; ok {
			msg.Body = body.Body
			msg.Attachments = body.Attachments
		}

		messages = append(messages, msg)
	}

	if bodyErr != nil {
		return messages, bodyErr
	}
	return messages, nil

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
