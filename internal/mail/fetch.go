package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersion/go-imap"
)

// defaultFetchBatchSize is the number of UIDs fetched per IMAP UID FETCH
// command when FetchOptions.BatchSize is not set (0).
const defaultFetchBatchSize = 10

// chunkUIDs splits uids into slices of at most size elements. A non-positive
// size returns the input as a single chunk (fetch everything in one command).
func chunkUIDs(uids []uint32, size int) [][]uint32 {
	if size <= 0 || len(uids) == 0 {
		return [][]uint32{uids}
	}
	chunks := make([][]uint32, 0, (len(uids)+size-1)/size)
	for i := 0; i < len(uids); i += size {
		end := i + size
		if end > len(uids) {
			end = len(uids)
		}
		chunks = append(chunks, uids[i:end])
	}
	return chunks
}

// fetchHeaders fetches envelope metadata and flags for the specified UIDs
// from the currently selected folder.
//
// UIDs are fetched in batches of at most batchSize per UID FETCH command to
// bound per-command memory and duration; pass batchSize <= 0 to fetch all
// UIDs in a single command. Returns raw IMAP messages containing envelope,
// flags, UID, and internal date. The caller is responsible for converting to
// domain Message values.
func (c *IMAPClient) fetchHeaders(ctx context.Context, uids []uint32, batchSize int) ([]*imap.Message, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("imap.fetch_headers: not connected")
	}
	if len(uids) == 0 {
		return nil, nil
	}

	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		imap.FetchInternalDate,
	}

	var messages []*imap.Message
	for _, chunk := range chunkUIDs(uids, batchSize) {
		seqset := new(imap.SeqSet)
		seqset.AddNum(chunk...)

		ch := make(chan *imap.Message, len(chunk))
		done := make(chan error, 1)

		go func() {
			done <- c.cli.UidFetch(seqset, items, ch)
		}()

		for msg := range ch {
			messages = append(messages, msg)
		}

		if err := <-done; err != nil {
			return nil, fmt.Errorf("imap.uid_fetch: %w", err)
		}
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
		// Strip any markup the sender embedded in the display name before
		// we wrap the address in our own angle brackets.
		name := SanitizeHeader(addr.PersonalName)
		return fmt.Sprintf("%s <%s@%s>", name, addr.MailboxName, addr.HostName)
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
