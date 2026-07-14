package mail

import (
	"strings"
	"testing"
)

func TestReadBody_PlainText(t *testing.T) {
	raw := "Subject: Test\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHello, this is a plain text body."
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	if body != "Hello, this is a plain text body." {
		t.Errorf("body = %q, want %q", body, "Hello, this is a plain text body.")
	}
}

func TestReadBody_PlainTextWithContentType(t *testing.T) {
	r := strings.NewReader("Hello, this is a plain text body.")

	body, attachments, err := readBody(r, "text/plain; charset=utf-8")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	if body != "Hello, this is a plain text body." {
		t.Errorf("body = %q, want %q", body, "Hello, this is a plain text body.")
	}
}

func TestReadBody_HTML(t *testing.T) {
	raw := "Subject: Test\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<html><body><h1>Title</h1><p>Some <b>bold</b> text.</p></body></html>"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
		if body != "Title\nSome bold text." {
			t.Errorf("body = %q, want %q", body, "Title\nSome bold text.")
		}
	}

func TestReadBody_MultipartAlternative(t *testing.T) {
	raw := "Subject: Multipart\r\n" +
		"Content-Type: multipart/alternative; boundary=\"=_boundary_123\"\r\n" +
		"\r\n" +
		"--=_boundary_123\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Plain text version.\r\n" +
		"\r\n" +
		"--=_boundary_123\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<html><body><p>HTML version.</p></body></html>\r\n" +
		"\r\n" +
		"--=_boundary_123--\r\n"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	// Should prefer text/plain over text/html.
	if body != "Plain text version." {
		t.Errorf("body = %q, want %q", body, "Plain text version.")
	}
}

func TestReadBody_MultipartAlternative_HTMLOnly(t *testing.T) {
	raw := "Subject: Multipart\r\n" +
		"Content-Type: multipart/alternative; boundary=\"=_boundary_123\"\r\n" +
		"\r\n" +
		"--=_boundary_123\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<html><body><p>HTML content.</p></body></html>\r\n" +
		"\r\n" +
		"--=_boundary_123--\r\n"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	if body != "HTML content." {
		t.Errorf("body = %q, want %q", body, "HTML content.")
	}
}

func TestReadBody_MultipartMixed_WithAttachment(t *testing.T) {
	raw := "Subject: With attachment\r\n" +
		"Content-Type: multipart/mixed; boundary=\"=_boundary_456\"\r\n" +
		"\r\n" +
		"--=_boundary_456\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Body text here.\r\n" +
		"\r\n" +
		"--=_boundary_456\r\n" +
		"Content-Type: application/pdf; name=\"report.pdf\"\r\n" +
		"Content-Disposition: attachment; filename=\"report.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"JVBERi0xLjQK\r\n" +
		"\r\n" +
		"--=_boundary_456--\r\n"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}

	if body != "Body text here." {
		t.Errorf("body = %q, want %q", body, "Body text here.")
	}

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Filename != "report.pdf" {
		t.Errorf("attachment filename = %q, want %q", attachments[0].Filename, "report.pdf")
	}
	if attachments[0].MIMEType != "application/pdf" {
		t.Errorf("attachment MIME type = %q, want %q", attachments[0].MIMEType, "application/pdf")
	}
	if attachments[0].Size <= 0 {
		t.Errorf("attachment size should be > 0, got %d", attachments[0].Size)
	}
}

func TestReadBody_MultipartMixed_MultipleAttachments(t *testing.T) {
	raw := "Subject: Multiple attachments\r\n" +
		"Content-Type: multipart/mixed; boundary=\"=_boundary_789\"\r\n" +
		"\r\n" +
		"--=_boundary_789\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Body with two attachments.\r\n" +
		"\r\n" +
		"--=_boundary_789\r\n" +
		"Content-Type: text/csv; name=\"data.csv\"\r\n" +
		"Content-Disposition: attachment; filename=\"data.csv\"\r\n" +
		"\r\n" +
		"a,b,c\r\n1,2,3\r\n" +
		"\r\n" +
		"--=_boundary_789\r\n" +
		"Content-Type: image/png; name=\"screenshot.png\"\r\n" +
		"Content-Disposition: attachment; filename=\"screenshot.png\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAA=\r\n" +
		"\r\n" +
		"--=_boundary_789--\r\n"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}

	if body != "Body with two attachments." {
		t.Errorf("body = %q, want %q", body, "Body with two attachments.")
	}

	if len(attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(attachments))
	}

	if attachments[0].Filename != "data.csv" {
		t.Errorf("attachment[0].Filename = %q, want %q", attachments[0].Filename, "data.csv")
	}
	if attachments[1].Filename != "screenshot.png" {
		t.Errorf("attachment[1].Filename = %q, want %q", attachments[1].Filename, "screenshot.png")
	}
}

func TestReadBody_InlineImage_NotAttachment(t *testing.T) {
	raw := "Subject: Inline image\r\n" +
		"Content-Type: multipart/related; boundary=\"=_boundary_abc\"\r\n" +
		"\r\n" +
		"--=_boundary_abc\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<img src=\"cid:image1\">\r\n" +
		"\r\n" +
		"--=_boundary_abc\r\n" +
		"Content-Type: image/png\r\n" +
		"Content-Disposition: inline\r\n" +
		"Content-ID: <image1>\r\n" +
		"\r\n" +
		"fakeimagecontent\r\n" +
		"\r\n" +
		"--=_boundary_abc--\r\n"
	r := strings.NewReader(raw)

	_, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}

	if attachments != nil {
		t.Errorf("expected no attachments for inline images, got %d", len(attachments))
	}
}

func TestReadBody_CharsetISO88591(t *testing.T) {
	// "Héllo Wörld" in ISO-8859-1
	rawBody := []byte{0x48, 0xe9, 0x6c, 0x6c, 0x6f, 0x20, 0x57, 0xf6, 0x72, 0x6c, 0x64}
	raw := "Subject: Charset test\r\n" +
		"Content-Type: text/plain; charset=iso-8859-1\r\n" +
		"\r\n"
	full := append([]byte(raw), rawBody...)
	r := strings.NewReader(string(full))

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	if body != "Héllo Wörld" {
		t.Errorf("body = %q, want %q", body, "Héllo Wörld")
	}
}

func TestReadBody_EmptyBody(t *testing.T) {
	raw := "Subject: Empty\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	if body != "" {
		t.Errorf("body = %q, want empty string", body)
	}
}

func TestReadBody_InvalidContentType(t *testing.T) {
	r := strings.NewReader("some text")

	_, _, err := readBody(r, "not-a-valid-content-type;;;")
	if err == nil {
		t.Fatal("expected error for invalid content type")
	}
}

func TestReadBody_AttachmentWithNameInContentType(t *testing.T) {
	raw := "Subject: Name in content type\r\n" +
		"Content-Type: multipart/mixed; boundary=\"=_boundary_name\"\r\n" +
		"\r\n" +
		"--=_boundary_name\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Body text.\r\n" +
		"\r\n" +
		"--=_boundary_name\r\n" +
		"Content-Type: application/octet-stream; name=\"data.bin\"\r\n" +
		"\r\n" +
		"binarydatahere\r\n" +
		"\r\n" +
		"--=_boundary_name--\r\n"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}

	if body != "Body text." {
		t.Errorf("body = %q, want %q", body, "Body text.")
	}
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Filename != "data.bin" {
		t.Errorf("attachment filename = %q, want %q", attachments[0].Filename, "data.bin")
	}
}

func TestReadBody_NestedMultipart(t *testing.T) {
	raw := "Subject: Nested\r\n" +
		"Content-Type: multipart/mixed; boundary=\"==outer\"\r\n" +
		"\r\n" +
		"--==outer\r\n" +
		"Content-Type: multipart/alternative; boundary=\"==inner\"\r\n" +
		"\r\n" +
		"--==inner\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Nested plain text.\r\n" +
		"\r\n" +
		"--==inner\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>Nested HTML.</p>\r\n" +
		"\r\n" +
		"--==inner--\r\n" +
		"\r\n" +
		"--==outer\r\n" +
		"Content-Type: application/zip; name=\"archive.zip\"\r\n" +
		"Content-Disposition: attachment; filename=\"archive.zip\"\r\n" +
		"\r\n" +
		"pkzipdata\r\n" +
		"\r\n" +
		"--==outer--\r\n"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}

	if body != "Nested plain text." {
		t.Errorf("body = %q, want %q", body, "Nested plain text.")
	}
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Filename != "archive.zip" {
		t.Errorf("attachment filename = %q, want %q", attachments[0].Filename, "archive.zip")
	}
	if attachments[0].MIMEType != "application/zip" {
		t.Errorf("attachment MIME type = %q, want %q", attachments[0].MIMEType, "application/zip")
	}
}

func TestReadBody_WithContentTypeOverride(t *testing.T) {
	// Pass a content-type override for a body without headers.
	r := strings.NewReader("<html><body><p>Override test</p></body></html>")

	body, attachments, err := readBody(r, "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	if body != "Override test" {
		t.Errorf("body = %q, want %q", body, "Override test")
	}
}

func TestReadBody_QuotedPrintableUTF8(t *testing.T) {
	raw := "Subject: QP\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"Hello =E2=9C=93 world\n"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	// =E2=9C=93 is the UTF-8 encoding of ✓ (check mark).
	if !strings.Contains(body, "✓") {
		t.Errorf("body should contain check mark, got %q", body)
	}
}

func TestReadBody_HTMLWithEntities(t *testing.T) {
	raw := "Subject: HTML entities\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>Price: &pound;10 &amp; &lt;5 &gt; 2</p>"
	r := strings.NewReader(raw)

	body, attachments, err := readBody(r, "")
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if attachments != nil {
		t.Errorf("expected no attachments, got %d", len(attachments))
	}
	if body != "Price: £10 & <5 > 2" {
		t.Errorf("body = %q, want %q", body, "Price: £10 & <5 > 2")
	}
}