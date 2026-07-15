package mail

import (
	"context"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

// BodyResult holds the extracted body text and attachment metadata for a
// single message after MIME parsing.
type BodyResult struct {
	Body        string
	Attachments []AttachmentMeta
}

// fetchBody fetches the full raw message bodies for the specified UIDs from
// the currently selected folder.
//
// It uses BODY.PEEK[] to avoid implicitly setting the \Seen flag. Each
// message is parsed from its raw RFC 2822 form, extracting the plain-text body
// and attachment metadata.
//
// Returns a map from UID to parsing result. UIDs whose bodies could not be
// fetched or parsed are omitted from the map; their errors are collected.
func (c *IMAPClient) fetchBody(ctx context.Context, uids []uint32) (map[uint32]BodyResult, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("imap.fetch_body: not connected")
	}
	if len(uids) == 0 {
		return nil, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)

	// BODY.PEEK[] — fetch the full message without setting \Seen.
	bodySection := &imap.BodySectionName{Peek: true}
	// items := []imap.FetchItem{bodySection.FetchItem()}
	items := []imap.FetchItem{imap.FetchUid, bodySection.FetchItem()}

	ch := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	go func() {
		done <- c.cli.UidFetch(seqset, items, ch)
	}()

	results := make(map[uint32]BodyResult, len(uids))
	var errs []string

	for msg := range ch {
		bodyLiteral := msg.GetBody(bodySection)
		if bodyLiteral == nil {
			errs = append(errs, fmt.Sprintf("uid %d: no body section returned", msg.Uid))
			continue
		}

		body, attachments, parseErr := readBody(bodyLiteral, "")
		if parseErr != nil {
			errs = append(errs, fmt.Sprintf("uid %d: parse body: %v", msg.Uid, parseErr))
		}

		results[msg.Uid] = BodyResult{
			Body:        body,
			Attachments: attachments,
		}
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap.uid_fetch_body: %w", err)
	}

	if len(errs) > 0 {
		return results, fmt.Errorf("imap.fetch_body: %d/%d messages had errors: %s",
			len(errs), len(uids), strings.Join(errs, "; "))
	}

	return results, nil
}

// readBody parses a MIME message read from r and extracts the plain-text body
// content and attachment metadata.
//
// The reader should contain a full RFC 2822 message (headers + body).
// The contentType parameter is used as a Content-Type override when the data
// has no headers of its own — pass the Content-Type header value or an empty
// string for auto-detection via message.Read.
//
// Text parts are extracted from the preferred text/plain alternative when
// available. text/html parts are included only when no text/plain part exists.
// Attachments are identified by Content-Disposition or a filename parameter.
func readBody(r io.Reader, contentType string) (string, []AttachmentMeta, error) { //nolint:gocyclo
	var entity *message.Entity
	var err error

	if contentType != "" {
		// Wrap the body in a header with the given content type so go-message
		// can handle charset conversion, transfer encoding, and multipart parsing.
		mediaType, params, pErr := mime.ParseMediaType(contentType)
		if pErr != nil {
			return "", nil, fmt.Errorf("readBody: invalid content type %q: %w", contentType, pErr)
		}

		var hdr message.Header
		hdr.SetContentType(mediaType, params)
		entity, err = message.New(hdr, r)
	} else {
		entity, err = message.Read(r)
	}

	if err != nil {
		// go-message returns partial entities on charset/encoding errors.
		if entity == nil {
			return "", nil, fmt.Errorf("readBody: %w", err)
		}
	}

	var bodyText strings.Builder
	var attachments []AttachmentMeta

	walkErr := entity.Walk(func(path []int, part *message.Entity, partErr error) error {
		if partErr != nil {
			// Continue — a single bad part shouldn't kill the whole parse.
			return nil
		}

		// Skip root multipart containers — only process leaf entities.
		mediaType, mediaParams, ctErr := part.Header.ContentType()
		if ctErr != nil {
			mediaType = "text/plain"
			mediaParams = nil
		}

		// Skip multipart containers; Walk will visit their children.
		if strings.HasPrefix(mediaType, "multipart/") {
			return nil
		}

		// Check for attachment disposition.
		disposition, dispParams, cdErr := part.Header.ContentDisposition()
		isAttachment := cdErr == nil && disposition == "attachment"
		hasFilename := false
		if cdErr == nil {
			_, hasFilename = dispParams["filename"]
		}
		if !isAttachment && !hasFilename {
			// Also check Content-Type name parameter (commonly used as filename).
			_, hasFilename = mediaParams["name"]
		}

		if isAttachment || hasFilename {
			filename := ""
			if cdErr == nil {
				if f, ok := dispParams["filename"]; ok {
					filename = f
				}
			}
			if filename == "" {
				if f, ok := mediaParams["name"]; ok {
					filename = f
				}
			}

			attachBody, rErr := io.ReadAll(part.Body)
			if rErr != nil {
				return nil // skip unreadable attachment
			}

			attachments = append(attachments, AttachmentMeta{
				Filename: filename,
				MIMEType: mediaType,
				Size:     int64(len(attachBody)),
			})
			return nil
		}

		// Only process text/* parts.
		if !strings.HasPrefix(mediaType, "text/") {
			return nil
		}

		attachBody, rErr := io.ReadAll(part.Body)
		if rErr != nil {
			return nil // skip unreadable text part
		}
		text := string(attachBody)

		// Prefer text/plain over text/html.
		subType := strings.TrimPrefix(mediaType, "text/")
		switch subType {
		case "plain":
			if bodyText.Len() == 0 {
				bodyText.WriteString(text)
			}
		case "html":
			if bodyText.Len() == 0 {
				bodyText.WriteString(StripHTML(text))
			}
		default:
			if bodyText.Len() == 0 {
				bodyText.WriteString(text)
			}
		}

		return nil
	})
	if walkErr != nil {
		return "", nil, fmt.Errorf("readBody.walk: %w", walkErr)
	}

	finalBody := bodyText.String()
	finalBody = StripControlChars(finalBody)
	finalBody = DecodeEntities(finalBody)
	finalBody = strings.TrimSpace(finalBody)

	if len(attachments) == 0 {
		attachments = nil
	}

	return finalBody, attachments, nil
}
