// Package mail provides the domain types and interfaces for the ingest
// service. It defines the core message model, classification results, and
// IMAP flag representations used throughout the pipeline.
//
// The composite key (account_label, uid) is the primary identifier for
// messages across all layers of the system.
package mail

import "time"

// ---------------------------------------------------------------------------
// Composite key
// ---------------------------------------------------------------------------

// MessageKey is the composite dedup key for a processed message.
type MessageKey struct {
	AccountLabel string
	UID          uint32
}

// Key returns the composite key string for dedup lookups.
func (k MessageKey) Key() string {
	return k.AccountLabel + "/" + itoa(k.UID)
}

// itoa is a small helper to avoid importing strconv for the Key method.
func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ---------------------------------------------------------------------------
// Message
// ---------------------------------------------------------------------------

// Message represents a single email fetched from an IMAP mailbox.
type Message struct {
	AccountLabel string
	UID          uint32
	Folder       string // e.g. "INBOX"
	Subject      string
	From         string
	To           string
	Date         time.Time
	Body         string // plain-text body, converted to UTF-8
	Attachments  []AttachmentMeta
	IsRead       bool // whether the \Seen flag was set on the server
}

// Key returns the composite dedup key for this message.
func (m Message) Key() MessageKey {
	return MessageKey{AccountLabel: m.AccountLabel, UID: m.UID}
}

// ---------------------------------------------------------------------------
// AttachmentMeta
// ---------------------------------------------------------------------------

// AttachmentMeta holds metadata about a single email attachment.
type AttachmentMeta struct {
	Filename string
	MIMEType string
	Size     int64 // bytes
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// Classification represents the LLM's classification result for a single
// message. The Key field links the result back to the original message.
type Classification struct {
	Key         MessageKey
	Label       string   // e.g. "Useful", "ToDelete", "Ads", or a custom label
	Confidence  float64  // 0.0 to 1.0
	Reason      string   // short justification from the LLM
	Summary     string   // concise summary of the email
	KeyPoints   []string // important facts or details from the email
	ActionItems []string // optional follow-up tasks requested by the email
	Urgency     string   // optional urgency indicator from the LLM
}

// ---------------------------------------------------------------------------
// Flag
// ---------------------------------------------------------------------------

// Flag represents an IMAP keyword flag to apply to a message.
//
// Keywords are plain, without backslash prefix: "Useful", "ToDelete", "Ads".
type Flag struct {
	Key     MessageKey
	Keyword string // IMAP keyword, e.g. "Useful", no backslash prefix
}
