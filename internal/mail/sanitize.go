package mail

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

// blockTags are HTML block-level elements that should be replaced with a
// newline to preserve text structure when stripping HTML.
var blockTags = map[string]bool{
	"p":          true,
	"div":        true,
	"br":         true,
	"hr":         true,
	"h1":         true,
	"h2":         true,
	"h3":         true,
	"h4":         true,
	"h5":         true,
	"h6":         true,
	"blockquote": true,
	"pre":        true,
	"table":      true,
	"tr":         true,
	"th":         true,
	"td":         true,
	"li":         true,
	"ol":         true,
	"ul":         true,
	"dl":         true,
	"dt":         true,
	"dd":         true,
	"section":    true,
	"article":    true,
	"nav":        true,
	"header":     true,
	"footer":     true,
	"aside":      true,
	"details":    true,
	"summary":    true,
	"figure":     true,
	"figcaption": true,
}

// StripHTML removes all HTML tags from s using a state machine. It handles:
//   - Opening and closing tags
//   - Self-closing tags
//   - HTML comments
//   - Script and style element content (removed entirely)
//   - DOCTYPE declarations
//   - CDATA sections
//   - Block-level elements replaced with newline for readability
//   - Malformed HTML (lone '<' or '<' followed by non-alpha emitted as text)
func StripHTML(s string) string { //nolint:gocyclo
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))

	type state int
	const (
		stateText    state = iota // copying text to output
		stateTag                  // inside < ... >
		stateScript               // inside <script> content
		stateStyle                // inside <style> content
		stateComment              // inside <!-- ... -->
		stateDoctype              // inside <!DOCTYPE ... >
	)

	st := stateText

	// readTagName extracts a lowercase tag name from a buffer starting just
	// after '<'. It returns the name.
	readTagName := func(s string) string {
		i := 0
		if i < len(s) && s[i] == '/' {
			i++
		}
		var n strings.Builder
		for i < len(s) {
			c := s[i]
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '-' && (c < '0' || c > '9') {
				break
			}
			n.WriteByte(c)
			i++
		}
		return strings.ToLower(n.String())
	}

	i := 0
	for i < len(s) {
		c := s[i]

		switch st {
		case stateText:
			if c == '<' {
				remaining := s[i+1:]

				if len(remaining) == 0 {
					// Lone '<' at end of string — emit as text.
					b.WriteByte(c)
					i++
					continue
				}

				// If the character after '<' is not a letter, '/', or '!',
				// this is probably a comparison operator, not a tag.
				next := remaining[0]
				if (next < 'a' || next > 'z') && (next < 'A' || next > 'Z') && next != '/' && next != '!' {
					b.WriteByte(c)
					i++
					continue
				}

				// HTML comment: <!--
				if len(remaining) >= 3 && remaining[:3] == "!--" {
					st = stateComment
					i += 4
					continue
				}

				// CDATA: <![CDATA[ — must be checked before the generic
				// DOCTYPE handler (<!...) since both start with "<!".
				if len(remaining) >= 8 && remaining[:8] == "![CDATA[" {
					// Find closing ]]>.
					end := strings.Index(s[i+3:], "]]>")
					if end >= 0 {
						// Emit the CDATA content as text. s[i+3:] starts
						// at "CDATA[...", so we skip 6 more chars for the
						// content after "CDATA[".
						b.WriteString(s[i+3+6 : i+3+end])
						i = i + 3 + end + 3
						continue
					}
					// Unclosed CDATA — just skip the tag.
					end = strings.IndexByte(s[i:], '>')
					if end >= 0 {
						i += end + 1
						continue
					}
					i++
					continue
				}

				// DOCTYPE: <!DOCTYPE or <!doctype
				if len(remaining) >= 1 && remaining[0] == '!' {
					st = stateDoctype
					i += 2
					continue
				}

				// Closing tag </...>
				if remaining[0] == '/' {
					name := readTagName(remaining)
					if name == "script" {
						i += 2 + len(name)
						for i < len(s) && s[i] != '>' {
							i++
						}
						if i < len(s) {
							i++
						}
						st = stateText
						continue
					}
					if name == "style" {
						i += 2 + len(name)
						for i < len(s) && s[i] != '>' {
							i++
						}
						if i < len(s) {
							i++
						}
						st = stateText
						continue
					}
					// Other closing tag.
					if blockTags[name] {
						b.WriteByte('\n')
					}
					i++
					for i < len(s) && s[i] != '>' {
						i++
					}
					if i < len(s) {
						i++ // skip '>'
					}
					continue
				}

				// Opening or self-closing tag <...> or <.../>
				tname := readTagName(remaining)

				if tname == "script" {
					st = stateScript
					i++
					for i < len(s) && s[i] != '>' {
						i++
					}
					if i < len(s) {
						i++
					}
					continue
				}

				if tname == "style" {
					st = stateStyle
					i++
					for i < len(s) && s[i] != '>' {
						i++
					}
					if i < len(s) {
						i++
					}
					continue
				}

				if blockTags[tname] {
					b.WriteByte('\n')
				}

				// Skip the entire tag.
				st = stateTag
				i++
				continue
			}

			// Normal text: write the character.
			b.WriteByte(c)
			i++

		case stateTag:
			if c == '>' {
				st = stateText
				i++
				continue
			}
			if c == '\'' || c == '"' {
				// Skip quoted attribute value to avoid misinterpreting '>'
				// inside quotes.
				quote := c
				i++
				for i < len(s) && s[i] != quote {
					i++
				}
				if i < len(s) {
					i++ // skip closing quote
				}
				continue
			}
			i++

		case stateScript:
			// Look for closing </script> tag (8 chars: </script).
			if c == '<' && i+8 < len(s) {
				closing := strings.ToLower(s[i : i+8])
				if closing == "</script" {
					i += 8
					for i < len(s) && s[i] != '>' {
						i++
					}
					if i < len(s) {
						i++
					}
					st = stateText
					continue
				}
			}
			i++

		case stateStyle:
			// Look for closing </style> tag (7 chars: </style).
			if c == '<' && i+7 < len(s) {
				closing := strings.ToLower(s[i : i+7])
				if closing == "</style" {
					i += 7
					for i < len(s) && s[i] != '>' {
						i++
					}
					if i < len(s) {
						i++
					}
					st = stateText
					continue
				}
			}
			i++

		case stateComment:
			// Look for closing -->.
			if c == '-' && i+2 < len(s) && s[i+1] == '-' && s[i+2] == '>' {
				i += 3
				st = stateText
				continue
			}
			i++

		case stateDoctype:
			if c == '>' {
				st = stateText
				i++
				continue
			}
			// Handle quoted strings inside doctype.
			if c == '\'' || c == '"' {
				quote := c
				i++
				for i < len(s) && s[i] != quote {
					i++
				}
				if i < len(s) {
					i++ // skip closing quote
				}
				continue
			}
			i++
		}
	}

	// Clean up: collapse multiple newlines, trim whitespace per line, then
	// collapse again (removing blank lines from trim), and trim overall.
	result := b.String()
	result = collapseNewlines(result)
	result = trimLines(result)
	result = collapseNewlines(result)
	result = strings.TrimSpace(result)
	return result
}

// collapseNewlines reduces runs of consecutive newlines to at most one.
func collapseNewlines(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevNL := false
	for _, r := range s {
		if r == '\n' {
			if !prevNL {
				b.WriteRune(r)
			}
			prevNL = true
		} else {
			b.WriteRune(r)
			prevNL = false
		}
	}
	return b.String()
}

// trimLines strips leading and trailing whitespace from each line in s.
func trimLines(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for {
		// Find the next newline (or end of string).
		idx := strings.IndexByte(s, '\n')
		var line string
		if idx < 0 {
			line = strings.TrimSpace(s)
			b.WriteString(line)
			break
		}
		line = strings.TrimSpace(s[:idx])
		if line != "" {
			b.WriteString(line)
		}
		b.WriteByte('\n')
		s = s[idx+1:]
	}
	return b.String()
}

// isControl reports whether r is a C0 or C1 control character (U+0000–U+001F,
// excluding whitespace; and U+007F–U+009F).
func isControl(r rune) bool {
	if r <= 0x1F {
		return r != '\t' && r != '\n' && r != '\r'
	}
	return 0x7F <= r && r <= 0x9F
}

// StripControlChars removes all C0 and C1 control characters from s, except
// for common whitespace (tab, newline, carriage return). It uses
// utf8.DecodeRuneInString to handle invalid UTF-8 bytes correctly — C1
// control bytes (0x80–0x9F) that are not part of valid UTF-8 sequences are
// stripped.
func StripControlChars(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte. Strip C1 control bytes (0x80-0x9F)
			// and the DEL character (0x7F); keep everything else.
			if s[i] >= 0x7F && s[i] <= 0x9F {
				i++
				continue
			}
			b.WriteByte(s[i])
			i++
			continue
		}
		if !isControl(r) {
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// htmlEntities is a minimal map of common HTML entities to their Unicode
// equivalents. It covers the most frequently encountered entities; rarer
// ones are left as-is.
var htmlEntities = map[string]string{
	"&amp;":     "&",
	"&lt;":      "<",
	"&gt;":      ">",
	"&quot;":    "\"",
	"&apos;":    "'",
	"&nbsp;":    " ",
	"&iexcl;":   "¡",
	"&cent;":    "¢",
	"&pound;":   "£",
	"&curren;":  "¤",
	"&yen;":     "¥",
	"&brvbar;":  "¦",
	"&sect;":    "§",
	"&uml;":     "¨",
	"&copy;":    "©",
	"&ordf;":    "ª",
	"&laquo;":   "«",
	"&not;":     "¬",
	"&shy;":     "\u00ad",
	"&reg;":     "®",
	"&macr;":    "¯",
	"&deg;":     "°",
	"&plusmn;":  "±",
	"&sup2;":    "²",
	"&sup3;":    "³",
	"&acute;":   "´",
	"&micro;":   "µ",
	"&para;":    "¶",
	"&middot;":  "·",
	"&cedil;":   "¸",
	"&sup1;":    "¹",
	"&ordm;":    "º",
	"&raquo;":   "»",
	"&frac14;":  "¼",
	"&frac12;":  "½",
	"&frac34;":  "¾",
	"&iquest;":  "¿",
	"&times;":   "×",
	"&divide;":  "÷",
	"&ndash;":   "–",
	"&mdash;":   "—",
	"&lsquo;":   "‘",
	"&rsquo;":   "’",
	"&sbquo;":   "‚",
	"&ldquo;":   "“",
	"&rdquo;":   "”",
	"&bdquo;":   "„",
	"&bull;":    "•",
	"&hellip;":  "…",
	"&prime;":   "′",
	"&Prime;":   "″",
	"&euro;":    "€",
	"&trade;":   "™",
	"&larr;":    "←",
	"&uarr;":    "↑",
	"&rarr;":    "→",
	"&darr;":    "↓",
	"&spades;":  "♠",
	"&clubs;":   "♣",
	"&hearts;":  "♥",
	"&diams;":   "♦",
}

// DecodeEntities replaces common HTML entities in s with their Unicode
// equivalents. It also handles numeric entities (both decimal &#NN; and
// hexadecimal &#xNN;).
func DecodeEntities(s string) string { //nolint:gocyclo
	if s == "" || !strings.ContainsRune(s, '&') {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}

		// Find the closing semicolon.
		semi := strings.IndexByte(s[i+1:], ';')
		if semi < 0 {
			// No semicolon — not an entity, emit as-is.
			b.WriteByte(s[i])
			i++
			continue
		}
		semi += i + 1 // absolute position of ';'

		entity := s[i : semi+1]

		// Check numeric entities.
		if len(entity) > 3 && entity[1] == '#' {
			var cp uint32
			if entity[2] == 'x' || entity[2] == 'X' {
				// Hex: &#xNN;
				for _, c := range entity[3 : len(entity)-1] {
					cp *= 16
					switch {
					case c >= '0' && c <= '9':
						cp += uint32(c - '0')
					case c >= 'a' && c <= 'f':
						cp += uint32(c - 'a' + 10)
					case c >= 'A' && c <= 'F':
						cp += uint32(c - 'A' + 10)
					}
				}
			} else {
				// Decimal: &#NN;
				for _, c := range entity[2 : len(entity)-1] {
					if c >= '0' && c <= '9' {
						cp = cp*10 + uint32(c-'0')
					}
				}
			}

			if cp > 0 && cp <= unicode.MaxRune && !unicode.Is(unicode.Cs, rune(cp)) {
				b.WriteRune(rune(cp))
				i = semi + 1
				continue
			}
			// Invalid numeric entity — emit as-is.
			b.WriteString(entity)
			i = semi + 1
			continue
		}

		// Named entity.
		if decoded, ok := htmlEntities[entity]; ok {
			b.WriteString(decoded)
			i = semi + 1
			continue
		}

		// Unknown entity — emit as-is.
		b.WriteString(entity)
		i = semi + 1
	}

	return b.String()
}

// ConvertCharset reads all content from r, decodes it from the charset
// specified in contentType (e.g. "text/plain; charset=ISO-8859-1"), and
// returns the result as a UTF-8 string. If contentType is empty or has no
// charset parameter, it defaults to UTF-8.
func ConvertCharset(r io.Reader, contentType string) (string, error) {
	// Read raw bytes first so we can handle empty input gracefully —
	// charset.NewReader may return an error on an empty reader.
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("mail.ConvertCharset: %w", err)
	}
	if len(raw) == 0 {
		return "", nil
	}

	ct := contentType
	if ct == "" {
		ct = "text/plain; charset=utf-8"
	}

	reader, err := charset.NewReader(strings.NewReader(string(raw)), ct)
	if err != nil {
		return "", fmt.Errorf("mail.ConvertCharset: %w", err)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("mail.ConvertCharset: %w", err)
	}

	return string(body), nil
}

// Truncate truncates s to at most limit runes, appending "…" if truncated.
// limit must be >= 0; values of 0 return an empty string.
func Truncate(s string, limit int) string {
	if limit < 0 {
		return ""
	}
	if limit == 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}

	// Need at least 1 rune for the ellipsis.
	if limit < 1 {
		return ""
	}

	var b strings.Builder
	b.WriteString(string(runes[:limit-1]))
	b.WriteRune('…')
	return b.String()
}