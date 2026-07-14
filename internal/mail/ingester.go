// Package mail provides domain types, the ingest interface, and MIME
// sanitization for the email AI agent.
package mail

import (
	"context"
	"time"

	"github.com/egorefimow/emailer/internal/config"
)

// Ingester fetches messages from an IMAP mailbox and applies keyword flags.
//
// Implementations maintain a single IMAP connection per account for the
// duration of a run and must be safe for sequential use within an account.
type Ingester interface {
	// Fetch retrieves messages from the given account's configured folders
	// within the time window specified by opts.
	Fetch(ctx context.Context, account config.IMAPAccount, opts FetchOptions) ([]Message, error)

	// ApplyFlags sets IMAP keyword flags on the specified messages.
	//
	// Flags are written as plain IMAP keywords (no backslash prefix) via
	// UID STORE. Errors for individual UIDs are logged but do not abort
	// the batch.
	ApplyFlags(ctx context.Context, account config.IMAPAccount, flags []Flag) error
}

// FetchOptions controls the scope of a fetch operation.
type FetchOptions struct {
	// Since is the start of the fetch window (inclusive). Messages with
	// internal dates on or after this time are candidates for ingestion.
	Since time.Time

	// FetchUnreadOnly restricts the search to messages without the \Seen
	// flag. When false (the default), all messages in the time window are
	// candidates and the SQLite store is relied upon for deduplication.
	FetchUnreadOnly bool

	// Folders limits the search to specific mailbox folders. If empty,
	// the account's configured folders are used.
	Folders []string
}