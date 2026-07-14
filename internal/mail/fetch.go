package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersion/go-imap"
)

// fetchHeaders fetches envelope metadata and flags for the specified UIDs
// from the currently selected folder.
//
// Returns raw IMAP messages containing envelope, flags, UID, and internal
// date. The caller is responsible for converting to domain Message values.
func (c *IMAPClient) fetchHeaders(ctx context.Context, uids []uint32) ([]*imap.Message, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("imap.fetch_headers: not connected")
	}
	if len(uids) == 0 {
		return nil, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)

	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		imap.FetchInternalDate,
	}

	ch := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	go func() {
		done <- c.cli.UidFetch(seqset, items, ch)
	}()

	var messages []*imap.Message
	for msg := range ch {
		messages = append(messages, msg)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap.uid_fetch: %w", err)
	}

	return messages, nil
}

// isRead reports whether the message has the \Seen flag set.
func isRead(flags []string) bool {
	for _, f := range flags {
		if f == imap.SeenFlag {
			return true
		}
	}
	return false
}

// formatAddress formats an IMAP address as a human-readable string.
// Returns "PersonalName <mailbox@host>" when personal name is present,
// otherwise just "mailbox@host".
func formatAddress(addr *imap.Address) string {
	if addr == nil {
		return ""
	}
	if addr.PersonalName != "" {
		return fmt.Sprintf("%s <%s@%s>", addr.PersonalName, addr.MailboxName, addr.HostName)
	}
	return fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
}

// formatAddressList formats a slice of IMAP addresses as a comma-separated
// human-readable string.
func formatAddressList(addrs []*imap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, formatAddress(a))
	}
	return strings.Join(parts, ", ")
}