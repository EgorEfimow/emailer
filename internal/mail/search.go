package mail

import (
	"context"
	"fmt"
	"time"

	"github.com/emersion/go-imap"
)

// selectFolder selects a mailbox folder for access. The folder is opened in
// read-only mode when the account is used for fetching only. Returns the
// mailbox status (e.g., message count, UID validity).
func (c *IMAPClient) selectFolder(ctx context.Context, folder string, readOnly bool) (*imap.MailboxStatus, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("imap.select_folder: not connected")
	}

	mbox, err := c.cli.Select(folder, readOnly)
	if err != nil {
		return nil, fmt.Errorf("imap.select_folder: %w", err)
	}
	return mbox, nil
}

// searchByWindow returns the UIDs of messages in the currently selected
// folder whose internal date falls on or after since.
//
// When unreadOnly is true, the search is further restricted to messages
// without the \Seen flag. The search operates on UIDs, not sequence
// numbers, so the returned values are stable across IMAP sessions.
func (c *IMAPClient) searchByWindow(ctx context.Context, since time.Time, unreadOnly bool) ([]uint32, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("imap.search_window: not connected")
	}

	criteria := newSearchWindowCriteria(since, unreadOnly)

	uids, err := c.cli.UidSearch(criteria)
	if err != nil {
		return nil, fmt.Errorf("imap.uid_search: %w", err)
	}
	return uids, nil
}

// newSearchWindowCriteria builds the IMAP search criteria for the fetch
// window. Extracted for testability.
func newSearchWindowCriteria(since time.Time, unreadOnly bool) *imap.SearchCriteria {
	criteria := imap.NewSearchCriteria()
	criteria.Since = since

	if unreadOnly {
		criteria.WithoutFlags = []string{"\\Seen"}
	}

	return criteria
}