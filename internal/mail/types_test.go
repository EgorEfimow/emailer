package mail

import (
	"testing"
)

func TestMessageKey_Equality(t *testing.T) {
	tests := []struct {
		name string
		a, b MessageKey
		want bool
	}{
		{
			name: "equal keys",
			a:    MessageKey{AccountLabel: "work", UID: 42},
			b:    MessageKey{AccountLabel: "work", UID: 42},
			want: true,
		},
		{
			name: "different account label",
			a:    MessageKey{AccountLabel: "work", UID: 42},
			b:    MessageKey{AccountLabel: "personal", UID: 42},
			want: false,
		},
		{
			name: "different UID",
			a:    MessageKey{AccountLabel: "work", UID: 42},
			b:    MessageKey{AccountLabel: "work", UID: 99},
			want: false,
		},
		{
			name: "both fields differ",
			a:    MessageKey{AccountLabel: "work", UID: 42},
			b:    MessageKey{AccountLabel: "personal", UID: 99},
			want: false,
		},
		{
			name: "zero value equality",
			a:    MessageKey{},
			b:    MessageKey{},
			want: true,
		},
		{
			name: "zero value vs non-zero",
			a:    MessageKey{},
			b:    MessageKey{AccountLabel: "a", UID: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a == tt.b
			if got != tt.want {
				t.Errorf("(%+v == %+v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestMessageKey_MapBehavior(t *testing.T) {
	m := make(map[MessageKey]string)

	m[MessageKey{AccountLabel: "work", UID: 1}] = "first"
	m[MessageKey{AccountLabel: "work", UID: 2}] = "second"
	m[MessageKey{AccountLabel: "personal", UID: 1}] = "third"

	// Retrieve existing keys.
	if v := m[MessageKey{AccountLabel: "work", UID: 1}]; v != "first" {
		t.Errorf("got %q, want %q", v, "first")
	}
	if v := m[MessageKey{AccountLabel: "work", UID: 2}]; v != "second" {
		t.Errorf("got %q, want %q", v, "second")
	}
	if v := m[MessageKey{AccountLabel: "personal", UID: 1}]; v != "third" {
		t.Errorf("got %q, want %q", v, "third")
	}

	// Retrieve a non-existent key.
	if v := m[MessageKey{AccountLabel: "work", UID: 99}]; v != "" {
		t.Errorf("expected empty string for missing key, got %q", v)
	}

	// Len should be 3.
	if len(m) != 3 {
		t.Errorf("map length = %d, want 3", len(m))
	}

	// Overwrite an existing key.
	m[MessageKey{AccountLabel: "work", UID: 1}] = "overwritten"
	if v := m[MessageKey{AccountLabel: "work", UID: 1}]; v != "overwritten" {
		t.Errorf("after overwrite got %q, want %q", v, "overwritten")
	}
	if len(m) != 3 {
		t.Errorf("map length after overwrite = %d, want 3", len(m))
	}
}

func TestMessageKey_KeyString(t *testing.T) {
	tests := []struct {
		name  string
		key   MessageKey
		want string
	}{
		{
			name:  "simple case",
			key:   MessageKey{AccountLabel: "work", UID: 42},
			want: "work/42",
		},
		{
			name:  "zero UID",
			key:   MessageKey{AccountLabel: "personal", UID: 0},
			want: "personal/0",
		},
		{
			name:  "large UID",
			key:   MessageKey{AccountLabel: "archive", UID: 4294967295},
			want: "archive/4294967295",
		},
		{
			name:  "empty label",
			key:   MessageKey{AccountLabel: "", UID: 1},
			want: "/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key.Key()
			if got != tt.want {
				t.Errorf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMessage_Key(t *testing.T) {
	m := Message{
		AccountLabel: "work",
		UID:          42,
		Subject:      "Hello",
		Folder:       "INBOX",
	}
	want := MessageKey{AccountLabel: "work", UID: 42}
	if got := m.Key(); got != want {
		t.Errorf("Message.Key() = %+v, want %+v", got, want)
	}
}