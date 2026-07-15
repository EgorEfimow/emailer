package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

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

	// logger is used for structured logging of IMAP operations and errors.
	logger *slog.Logger
	// timeout bounds each IMAP command via client.Timeout (0 = no timeout).
	timeout time.Duration
}

// NewIMAPClient returns a new IMAPClient ready for connection.
//
// logger must be non-nil; it is used for structured logging throughout the
// client's lifetime. timeout bounds each IMAP command (0 means no timeout).
func NewIMAPClient(logger *slog.Logger, timeout time.Duration) (*IMAPClient, error) {
	if logger == nil {
		return nil, fmt.Errorf("mail.NewIMAPClient: logger must not be nil")
	}
	return &IMAPClient{
		logger:  logger,
		timeout: timeout,
	}, nil
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
			if logoutErr := cli.Logout(); logoutErr != nil {
				return fmt.Errorf("imap.starttls: %w (logout: %w)", err, logoutErr)
			}
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

	c.cli.Timeout = c.timeout

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

// reconnect tears down any existing connection and establishes a fresh one
// for the given account (Dial + Login). It is used to recover from a dropped
// connection. The operation is bound to ctx.
func (c *IMAPClient) reconnect(ctx context.Context, account config.IMAPAccount) error {
	if c.cli != nil {
		// Best-effort logout of the dead connection; the error is expected
		// when the connection is already broken, so just record it.
		if err := c.cli.Logout(); err != nil {
			c.logger.Debug("imap.reconnect: logout of stale connection failed",
				slog.String("account", account.Label),
				slog.Any("error", err),
			)
		}
		c.cli = nil
	}

	if err := c.Dial(ctx, account); err != nil {
		c.logger.Error("imap.reconnect: dial failed",
			slog.String("account", account.Label),
			slog.Any("error", err),
		)
		return fmt.Errorf("imap.reconnect.dial: %w", err)
	}
	if err := c.Login(ctx, account); err != nil {
		c.logger.Error("imap.reconnect: login failed",
			slog.String("account", account.Label),
			slog.Any("error", err),
		)
		return fmt.Errorf("imap.reconnect.login: %w", err)
	}
	return nil
}

// isConnectionError reports whether err indicates a dropped or unusable IMAP
// connection, as opposed to a semantic IMAP error (e.g. a NO response for a
// single command). Such errors warrant a reconnect-and-retry.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection") ||
		strings.Contains(msg, "closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed") ||
		strings.Contains(msg, "timeout")
}

// withReconnect runs op, and if it fails due to a dropped connection (or if
// the client is not yet connected) it reconnects once using the account
// credentials and retries op exactly once. Non-connection errors are returned
// unchanged without a retry. All network activity is bound to ctx.
func (c *IMAPClient) withReconnect(ctx context.Context, account config.IMAPAccount, op func() error) error {
	if c.cli == nil {
		if rErr := c.reconnect(ctx, account); rErr != nil {
			return fmt.Errorf("imap.connect: %w", rErr)
		}
	}

	err := op()
	if err == nil {
		return nil
	}
	if !isConnectionError(err) {
		return err
	}

	c.logger.Warn("imap: command failed with connection error; reconnecting",
		slog.String("account", account.Label),
		slog.Any("error", err),
	)

	if rErr := c.reconnect(ctx, account); rErr != nil {
		return fmt.Errorf("imap.reconnect: %w (original: %v)", rErr, err)
	}

	// Single retry on the freshly established connection.
	if retryErr := op(); retryErr != nil {
		return retryErr
	}
	return nil
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

		var folderMessages []Message
		fetchErr := c.withReconnect(ctx, account, func() error {
			var err error
			folderMessages, err = c.fetchFolder(ctx, account, folder, opts)
			return err
		})
		if fetchErr != nil {
			c.logger.Warn("imap.fetch: folder failed",
				slog.String("account", account.Label),
				slog.String("folder", folder),
				slog.Any("error", fetchErr),
			)
			errs = append(errs, fmt.Sprintf("folder %q: %v", folder, fetchErr))
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

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultFetchBatchSize
	}

	headers, err := c.fetchHeaders(ctx, uids, batchSize)
	if err != nil {
		return nil, err
	}

	bodies, bodyErr := c.fetchBody(ctx, uids, batchSize)

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
			msg.Subject = SanitizeHeader(header.Envelope.Subject)
			msg.From = SanitizeAddressField(formatAddressList(header.Envelope.From))
			msg.To = SanitizeAddressField(formatAddressList(header.Envelope.To))
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

	// Reconnect once on a connection-level failure (or if not yet connected).
	return c.withReconnect(ctx, account, func() error {
		return c.applyFlagsOnce(ctx, account, flags)
	})
}

// applyFlagsOnce performs a single attempt at storing flags. It must only be
// called on a live connection; withReconnect guarantees that.
func (c *IMAPClient) applyFlagsOnce(ctx context.Context, account config.IMAPAccount, flags []Flag) error {
	if c.cli == nil {
		return fmt.Errorf("imap.apply_flags: not connected")
	}

	// Select INBOX in read-write mode (must not be read-only to modify flags).
	// TODO: support per-folder flag application when folder info is added to Flag.
	// The returned mailbox status carries PERMANENTFLAGS, which we consult
	// before attempting to store custom keywords.
	mbox, err := c.selectFolder(ctx, "INBOX", false)
	if err != nil {
		return fmt.Errorf("imap.apply_flags.select: %w", err)
	}

	// Group flags by keyword for efficient IMAP calls.
	byKeyword := make(map[string][]uint32)
	for _, f := range flags {
		byKeyword[f.Keyword] = append(byKeyword[f.Keyword], f.Key.UID)
	}

	var errs []string
	var skipped int
	for keyword, uids := range byKeyword {
		if !keywordAllowed(mbox.PermanentFlags, keyword) {
			skipped++
			c.logger.Warn("imap.apply_flags: server does not permit keyword; skipping",
				slog.String("account", account.Label),
				slog.String("folder", "INBOX"),
				slog.String("keyword", keyword),
			)
			continue
		}

		seqset := new(imap.SeqSet)
		seqset.AddNum(uids...)

		if err := c.cli.UidStore(seqset, imap.AddFlags, []interface{}{keyword}, nil); err != nil {
			c.logger.Warn("imap.apply_flags: store failed",
				slog.String("account", account.Label),
				slog.String("folder", "INBOX"),
				slog.String("keyword", keyword),
				slog.Int("uids", len(uids)),
				slog.Any("error", err),
			)
			errs = append(errs, fmt.Sprintf("keyword %q on %d UIDs: %v", keyword, len(uids), err))
		}
	}

	if skipped > 0 && skipped == len(byKeyword) {
		// Every keyword was rejected by the server — flagging is impossible,
		// surface it so the caller can decide how to handle the gap.
		return fmt.Errorf("imap.apply_flags: %d/%d keywords not permitted by server PERMANENTFLAGS", skipped, len(byKeyword))
	}

	if len(errs) > 0 {
		return fmt.Errorf("imap.apply_flags: %d/%d groups failed: %s",
			len(errs), len(byKeyword), strings.Join(errs, "; "))
	}

	return nil
}

// keywordAllowed reports whether the IMAP server permits storing the given
// custom keyword, based on the selected folder's PERMANENTFLAGS.
//
// A "*" entry (TryCreateFlag) means the server accepts arbitrary keywords;
// otherwise the keyword must be explicitly listed. System flags (those with a
// leading backslash) are always permitted. When the server does not advertise
// PERMANENTFLAGS (empty list), we cannot conclude the keyword is unsupported,
// so we attempt the store and let the server reject it.
func keywordAllowed(permanentFlags []string, keyword string) bool {
	if strings.HasPrefix(keyword, "\\") {
		return true
	}
	if len(permanentFlags) == 0 {
		return true
	}
	for _, f := range permanentFlags {
		if f == "\\*" {
			return true
		}
		if strings.EqualFold(f, keyword) {
			return true
		}
	}
	return false
}
