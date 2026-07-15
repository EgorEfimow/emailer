package mail

import (
	"strings"
	"testing"
)

func TestStripHTML_Empty(t *testing.T) {
	if got := StripHTML(""); got != "" {
		t.Errorf("StripHTML(\"\") = %q, want empty", got)
	}
}

func TestStripHTML_PlainText(t *testing.T) {
	input := "Hello, world! This is plain text."
	if got := StripHTML(input); got != input {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, input)
	}
}

func TestStripHTML_SimpleTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bold tag",
			input: "Hello <b>world</b>",
			want:  "Hello world",
		},
		{
			name:  "anchor tag",
			input: "Click <a href=\"https://example.com\">here</a>",
			want:  "Click here",
		},
		{
			name:  "span with style",
			input: "Text <span style=\"color:red\">with</span> span",
			want:  "Text with span",
		},
		{
			name:  "multiple tags",
			input: "A <b>bold</b> and <i>italic</i> word",
			want:  "A bold and italic word",
		},
		{
			name:  "tag with attributes",
			input: "<div class=\"container\" id=\"main\">content</div>",
			want:  "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_SelfClosingTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "br tag",
			input: "line1<br>line2",
			want:  "line1\nline2",
		},
		{
			name:  "br tag with slash",
			input: "line1<br/>line2",
			want:  "line1\nline2",
		},
		{
			name:  "hr tag",
			input: "before<hr>after",
			want:  "before\nafter",
		},
		{
			name:  "img tag (not block, no newline)",
			input: "text<img src=\"pic.png\" alt=\"a\">text",
			want:  "texttext",
		},
		{
			name:  "input tag (not block, no newline)",
			input: "before<input type=\"text\">after",
			want:  "beforeafter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_NestedTags(t *testing.T) {
	input := "<div><p>Hello <b>world</b></p></div>"
	want := "Hello world"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}

func TestStripHTML_BlockLevelTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "paragraphs",
			input: "<p>First paragraph</p><p>Second paragraph</p>",
			want:  "First paragraph\nSecond paragraph",
		},
		{
			name:  "headings",
			input: "<h1>Title</h1><h2>Subtitle</h2>",
			want:  "Title\nSubtitle",
		},
		{
			name:  "list items",
			input: "<ul><li>Item one</li><li>Item two</li></ul>",
			want:  "Item one\nItem two",
		},
		{
			name:  "blockquote",
			input: "<blockquote>Cited text</blockquote>",
			want:  "Cited text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_Comments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple comment",
			input: "before<!-- comment -->after",
			want:  "beforeafter",
		},
		{
			name:  "comment with tags inside",
			input: "text<!-- <div>hidden</div> -->visible",
			want:  "textvisible",
		},
		{
			name:  "multi-line comment",
			input: "a<!--\nmulti\nline\n-->b",
			want:  "ab",
		},
		{
			name:  "comment with dashes",
			input: "x<!-- -- -- -->y",
			want:  "xy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_ScriptTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "script with content removed",
			input: "before<script>alert('xss')</script>after",
			want:  "beforeafter",
		},
		{
			name:  "script with type attribute",
			input: "text<script type=\"text/javascript\">var x = 1;</script>more",
			want:  "textmore",
		},
		{
			name:  "script with src attribute",
			input: "a<script src=\"evil.js\"></script>b",
			want:  "ab",
		},
		{
			name:  "multi-line script",
			input: "x<script>\nfunction f() {\n  return 1;\n}\n</script>y",
			want:  "xy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_StyleTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "style content removed",
			input: "before<style>body { color: red; }</style>after",
			want:  "beforeafter",
		},
		{
			name:  "style with type",
			input: "text<style type=\"text/css\">.cls {}</style>more",
			want:  "textmore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_MalformedHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "lone less-than",
			input: "a < b > c",
			want:  "a < b > c",
		},
		{
			name:  "unclosed tag at end",
			input: "hello <world",
			// <world starts with a letter, so it's treated as a tag.
			want:  "hello",
		},
		{
			name:  "unclosed tag mid-text",
			input: "hello <world more",
			// <world starts with a letter, so it's treated as a tag.
			want:  "hello",
		},
		{
			name:  "extra closing bracket",
			input: "a<b>c</b>d>e",
			want:  "acd>e",
		},
		{
			name:  "tag with no closing bracket",
			input: "a<b>c</b",
			want:  "ac",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_QuotedAttributes(t *testing.T) {
	// Ensure '>' inside quoted attribute values is not mistaken for closing tag.
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "greater-than in double-quoted attr",
			input: "<a title=\"a > b\">link</a>",
			want:  "link",
		},
		{
			name:  "greater-than in single-quoted attr",
			input: "<a title='a > b'>link</a>",
			want:  "link",
		},
		{
			name:  "multiple attributes with >",
			input: "<div data-x=\"a > b\" data-y=\"c < d\">text</div>",
			want:  "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_Doctype(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "HTML5 doctype",
			input: "<!DOCTYPE html><html><body>text</body></html>",
			want:  "text",
		},
		{
			name:  "HTML4 doctype with URL",
			input: "<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.01//EN\" \"http://www.w3.org/TR/html4/strict.dtd\">content",
			want:  "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTML_CollapseNewlines(t *testing.T) {
	input := "<p>a</p>\n\n\n<p>b</p>"
	want := "a\nb"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}

func TestStripHTML_TrimSpaces(t *testing.T) {
	// Leading/trailing whitespace from block tags should be trimmed.
	input := "  <p>hello</p>  "
	want := "hello"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}

func TestStripHTML_RealisticEmailHTML(t *testing.T) {
	input := `<!DOCTYPE html>
<html>
<head><style>body { font-family: Arial; }</style></head>
<body>
  <div class="header">
    <h1>Monthly Report</h1>
    <p>Date: <span class="date">2024-01-15</span></p>
  </div>
  <div class="content">
    <p>Dear <b>User</b>,</p>
    <p>Your account has <a href="https://example.com">5 new messages</a>.</p>
    <!-- This is a comment -->
    <ul>
      <li>Item 1</li>
      <li>Item 2</li>
    </ul>
  </div>
  <script>
    console.log('hidden');
  </script>
</body>
</html>`
	// We expect the text content with block-level structure preserved.
	want := "Monthly Report\nDate: 2024-01-15\nDear User,\nYour account has 5 new messages.\nItem 1\nItem 2"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML realistic email:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestStripHTML_NoNewlineForInlineTags(t *testing.T) {
	input := "Hello <b>world</b> and <i>friends</i>"
	want := "Hello world and friends"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}

func TestStripHTML_CDATA(t *testing.T) {
	// Note: CDATA is uncommon in HTML5 but appears in XHTML.
	input := "before<![CDATA[some <content> here]]>after"
	want := "beforesome <content> hereafter"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}

func TestStripHTML_TagWithHyphen(t *testing.T) {
	// Custom elements with hyphens (e.g., <my-component>)
	input := "text<my-component attr=\"val\">content</my-component>more"
	want := "textcontentmore"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}

func TestStripHTML_UpperCaseTags(t *testing.T) {
	input := "<P>paragraph</P> with <B>bold</B>"
	// <P> is a block tag, so it produces a newline separator.
	want := "paragraph\nwith bold"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}

func TestStripHTML_MixedCaseScript(t *testing.T) {
	input := "a<SCRIPT>alert(1)</SCRIPT>b"
	want := "ab"
	got := StripHTML(input)
	if got != want {
		t.Errorf("StripHTML(%q) = %q, want %q", input, got, want)
	}
}
func TestSanitizeHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "plain", input: "Hello world", want: "Hello world"},
		{name: "html tags stripped", input: "Sale <b>50%</b> off", want: "Sale 50% off"},
		{name: "script removed", input: "Hi<script>alert(1)</script> there", want: "Hi there"},
		{name: "entities decoded", input: "AT&amp;T &lt;price&gt;", want: "AT&T <price>"},
		{name: "control chars stripped", input: "a\x00b\x1Bc", want: "abc"},
		{
			name:  "block tags collapsed",
			input: "<p>Quarterly</p><p>Report</p>",
			want:  "Quarterly\nReport",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeHeader(tt.input); got != tt.want {
				t.Errorf("SanitizeHeader(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeAddressField(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "plain address", input: "Bob <bob@example.com>", want: "Bob <bob@example.com>"},
		{name: "control chars stripped", input: "Bob\x01 <bob@example.com>", want: "Bob <bob@example.com>"},
		{name: "entities decoded in name", input: "AT&amp;T <a@b.com>", want: "AT&T <a@b.com>"},
		{
			name:  "angle-bracket address preserved",
			input: "Name <a@b.com>",
			want:  "Name <a@b.com>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeAddressField(tt.input); got != tt.want {
				t.Errorf("SanitizeAddressField(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripControlChars_Empty(t *testing.T) {
	if got := StripControlChars(""); got != "" {
		t.Errorf("StripControlChars(\"\") = %q, want empty", got)
	}
}

func TestStripControlChars_NoControls(t *testing.T) {
	input := "Hello, world! Normal text with numbers 123."
	if got := StripControlChars(input); got != input {
		t.Errorf("StripControlChars(%q) = %q, want %q", input, got, input)
	}
}

func TestStripControlChars_RemovesC0(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "null byte", input: "a\x00b", want: "ab"},
		{name: "bell character", input: "a\x07b", want: "ab"},
		{name: "escape character", input: "a\x1Bb", want: "ab"},
		{name: "multiple controls", input: "\x00\x01\x02abc\x1F", want: "abc"},
		{name: "all whitespace preserved", input: "a\tb\nc\rd", want: "a\tb\nc\rd"},
		{name: "tab preserved, null removed", input: "a\x00\tb", want: "a\tb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripControlChars(tt.input)
			if got != tt.want {
				t.Errorf("StripControlChars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripControlChars_RemovesC1(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "delete character", input: "a\x7Fb", want: "ab"},
		{name: "C1 range start", input: "a\x80b", want: "ab"},
		{name: "C1 range end", input: "a\x9Fb", want: "ab"},
		{name: "multiple C1 controls", input: "\x80\x90\x9Aabc", want: "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripControlChars(tt.input)
			if got != tt.want {
				t.Errorf("StripControlChars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripControlChars_MixedContent(t *testing.T) {
	input := "Hello\x00 \x1BWorld\x7F!"
	want := "Hello World!"
	got := StripControlChars(input)
	if got != want {
		t.Errorf("StripControlChars(%q) = %q, want %q", input, got, want)
	}
}

func TestDecodeEntities_Empty(t *testing.T) {
	if got := DecodeEntities(""); got != "" {
		t.Errorf("DecodeEntities(\"\") = %q, want empty", got)
	}
}

func TestDecodeEntities_NoAmpersand(t *testing.T) {
	input := "Hello, world! No entities here."
	if got := DecodeEntities(input); got != input {
		t.Errorf("DecodeEntities(%q) = %q, want %q", input, got, input)
	}
}

func TestDecodeEntities_NamedEntities(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "ampersand", input: "AT&amp;T", want: "AT&T"},
		{name: "less than", input: "a &lt; b", want: "a < b"},
		{name: "greater than", input: "a &gt; b", want: "a > b"},
		{name: "quotation mark", input: "He said &quot;hello&quot;", want: `He said "hello"`},
		{name: "apostrophe", input: "It&apos;s fine", want: "It's fine"},
		{name: "non-breaking space", input: "a&nbsp;b", want: "a b"},
		{name: "copyright", input: "&copy; 2024", want: "© 2024"},
		{name: "multiple entities", input: "&lt;div&gt;hello&lt;/div&gt;", want: "<div>hello</div>"},
		{name: "euro symbol", input: "Price: &euro;10", want: "Price: €10"},
		{name: "trade mark", input: "Brand&trade;", want: "Brand™"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeEntities(tt.input)
			if got != tt.want {
				t.Errorf("DecodeEntities(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeEntities_NumericDecimal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "decimal A", input: "&#65;", want: "A"},
		{name: "decimal a", input: "&#97;", want: "a"},
		{name: "decimal copyright", input: "&#169;", want: "©"},
		{name: "decimal in text", input: "Hello &#65; &#66; &#67;", want: "Hello A B C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeEntities(tt.input)
			if got != tt.want {
				t.Errorf("DecodeEntities(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeEntities_NumericHex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "hex A", input: "&#x41;", want: "A"},
		{name: "hex lowercase", input: "&#x61;", want: "a"},
		{name: "hex copyright", input: "&#xA9;", want: "©"},
		{name: "hex uppercase X", input: "&#X41;", want: "A"},
		{name: "hex emoji", input: "&#x1F600;", want: "\U0001F600"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeEntities(tt.input)
			if got != tt.want {
				t.Errorf("DecodeEntities(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeEntities_UnknownEntity(t *testing.T) {
	input := "Hello &unknown; world"
	want := "Hello &unknown; world"
	got := DecodeEntities(input)
	if got != want {
		t.Errorf("DecodeEntities(%q) = %q, want %q", input, got, want)
	}
}

func TestDecodeEntities_AmpersandWithoutSemicolon(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "lone ampersand", input: "AT&T", want: "AT&T"},
		{name: "ampersand at end", input: "hello &", want: "hello &"},
		{name: "ampersand with trailing space", input: "A & B", want: "A & B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeEntities(tt.input)
			if got != tt.want {
				t.Errorf("DecodeEntities(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeEntities_DoubleDecode(t *testing.T) {
	input := "&amp;lt;"
	want := "&lt;"
	got := DecodeEntities(input)
	if got != want {
		t.Errorf("DecodeEntities(%q) = %q, want %q", input, got, want)
	}
}

func TestDecodeEntities_AdjacentEntities(t *testing.T) {
	input := "&lt;&gt;&amp;"
	want := "<>&"
	got := DecodeEntities(input)
	if got != want {
		t.Errorf("DecodeEntities(%q) = %q, want %q", input, got, want)
	}
}

func TestConvertCharset_ISO8859_1(t *testing.T) {
	// "Héllò" in ISO-8859-1: H=0x48, é=0xE9, l=0x6C, l=0x6C, ò=0xF2
	input := []byte{0x48, 0xE9, 0x6C, 0x6C, 0xF2}
	r := strings.NewReader(string(input))
	got, err := ConvertCharset(r, "text/plain; charset=ISO-8859-1")
	if err != nil {
		t.Fatalf("ConvertCharset() unexpected error: %v", err)
	}
	want := "Héllò"
	if got != want {
		t.Errorf("ConvertCharset(ISO-8859-1) = %q, want %q", got, want)
	}
}

func TestConvertCharset_KOI8R(t *testing.T) {
	// "Привет" in KOI8-R: П=0xF0, р=0xD2, и=0xC9, в=0xD7, е=0xC5, т=0xD4
	input := []byte{0xF0, 0xD2, 0xC9, 0xD7, 0xC5, 0xD4}
	r := strings.NewReader(string(input))
	got, err := ConvertCharset(r, "text/plain; charset=KOI8-R")
	if err != nil {
		t.Fatalf("ConvertCharset() unexpected error: %v", err)
	}
	want := "Привет"
	if got != want {
		t.Errorf("ConvertCharset(KOI8-R) = %q, want %q", got, want)
	}
}

func TestConvertCharset_ShiftJIS(t *testing.T) {
	// "日本語" in Shift-JIS: 日=0x93FA, 本=0x967B, 語=0x8CEA
	input := []byte{0x93, 0xFA, 0x96, 0x7B, 0x8C, 0xEA}
	r := strings.NewReader(string(input))
	got, err := ConvertCharset(r, "text/plain; charset=Shift_JIS")
	if err != nil {
		t.Fatalf("ConvertCharset() unexpected error: %v", err)
	}
	want := "日本語"
	if got != want {
		t.Errorf("ConvertCharset(Shift_JIS) = %q, want %q", got, want)
	}
}

func TestConvertCharset_EmptyContentType(t *testing.T) {
	input := "Hello, UTF-8 world! 🌍"
	r := strings.NewReader(input)
	got, err := ConvertCharset(r, "")
	if err != nil {
		t.Fatalf("ConvertCharset() unexpected error: %v", err)
	}
	if got != input {
		t.Errorf("ConvertCharset(empty) = %q, want %q", got, input)
	}
}

func TestConvertCharset_ExplicitUTF8(t *testing.T) {
	input := "Hello, UTF-8! "
	r := strings.NewReader(input)
	got, err := ConvertCharset(r, "text/plain; charset=utf-8")
	if err != nil {
		t.Fatalf("ConvertCharset() unexpected error: %v", err)
	}
	if got != input {
		t.Errorf("ConvertCharset(utf-8) = %q, want %q", got, input)
	}
}

func TestConvertCharset_EmptyReader(t *testing.T) {
	r := strings.NewReader("")
	got, err := ConvertCharset(r, "text/plain; charset=utf-8")
	if err != nil {
		t.Fatalf("ConvertCharset() unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("ConvertCharset(empty) = %q, want empty", got)
	}
}

func TestConvertCharset_ContentTypeWithoutCharset(t *testing.T) {
	// No charset parameter — should default to UTF-8.
	input := "default utf-8"
	r := strings.NewReader(input)
	got, err := ConvertCharset(r, "text/plain")
	if err != nil {
		t.Fatalf("ConvertCharset() unexpected error: %v", err)
	}
	if got != input {
		t.Errorf("ConvertCharset(no charset) = %q, want %q", got, input)
	}
}

func TestTruncate_Empty(t *testing.T) {
	if got := Truncate("", 10); got != "" {
		t.Errorf("Truncate(\"\", 10) = %q, want empty", got)
	}
}

func TestTruncate_NegativeLimit(t *testing.T) {
	if got := Truncate("hello", -1); got != "" {
		t.Errorf("Truncate(\"hello\", -1) = %q, want empty", got)
	}
}

func TestTruncate_ZeroLimit(t *testing.T) {
	if got := Truncate("hello", 0); got != "" {
		t.Errorf("Truncate(\"hello\", 0) = %q, want empty", got)
	}
}

func TestTruncate_ShorterThanLimit(t *testing.T) {
	input := "hello"
	want := "hello"
	got := Truncate(input, 10)
	if got != want {
		t.Errorf("Truncate(%q, 10) = %q, want %q", input, got, want)
	}
}

func TestTruncate_ExactLimit(t *testing.T) {
	input := "hello"
	want := "hello"
	got := Truncate(input, 5)
	if got != want {
		t.Errorf("Truncate(%q, 5) = %q, want %q", input, got, want)
	}
}

func TestTruncate_LongerThanLimit(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{name: "basic truncation", input: "hello world", limit: 6, want: "hello…"},
		{name: "truncate to 1", input: "hello", limit: 1, want: "…"},
		{name: "limit 2", input: "hello", limit: 2, want: "h…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.input, tt.limit)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.limit, got, tt.want)
			}
		})
	}
}

func TestTruncate_RuneAware(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{
			name:  "CJK characters truncated by rune count",
			// 8 runes: a, 世, b, 界, c, d, e
			input: "a世b界cde",
			limit: 4,
			want:  "a世b…",
		},
		{
			name:  "Cyrillic truncation",
			input: "Привет мир",
			limit: 7,
			want:  "Привет…",
		},
		{
			name:  "mixed scripts exactly at limit",
			input: "a世b界",
			limit: 4,
			want:  "a世b界",
		},
		{
			name:  "emoji truncation",
			input: "a😀b😎c",
			limit: 3,
			want:  "a😀…",
		},
		{
			name:  "combining characters",
			// e + combining acute accent (U+0301) = é, each is a separate rune
			input: "café latte",
			limit: 6,
			want:  "café…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.input, tt.limit)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.limit, got, tt.want)
			}
		})
	}
}
