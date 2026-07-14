package mail

import (
	"context"
	"testing"

	"github.com/emersion/go-imap"
)

func TestFetchHeaders_NotConnected(t *testing.T) {
	c := NewIMAPClient()

	_, err := c.fetchHeaders(context.Background(), []uint32{1, 2, 3})
	if err == nil {
		t.Fatal("expected error from fetchHeaders when not connected")
	}
}

func TestIsRead(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		want  bool
	}{
		{
			name:  "has seen flag",
			flags: []string{imap.SeenFlag},
			want:  true,
		},
		{
			name:  "multiple flags with seen",
			flags: []string{"\\Answered", imap.SeenFlag, "\\Deleted"},
			want:  true,
		},
		{
			name:  "no seen flag",
			flags: []string{"\\Answered", "\\Deleted"},
			want:  false,
		},
		{
			name:  "empty flags",
			flags: nil,
			want:  false,
		},
		{
			name:  "only recent flag",
			flags: []string{"\\Recent"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRead(tt.flags)
			if got != tt.want {
				t.Errorf("isRead(%v) = %v, want %v", tt.flags, got, tt.want)
			}
		})
	}
}

func TestFormatAddress(t *testing.T) {
	tests := []struct {
		name string
		addr *imap.Address
		want string
	}{
		{
			name: "nil address",
			addr: nil,
			want: "",
		},
		{
			name: "with personal name",
			addr: &imap.Address{
				PersonalName: "John Doe",
				MailboxName:  "john",
				HostName:     "example.com",
			},
			want: "John Doe <john@example.com>",
		},
		{
			name: "without personal name",
			addr: &imap.Address{
				PersonalName: "",
				MailboxName:  "jane",
				HostName:     "test.org",
			},
			want: "jane@test.org",
		},
		{
			name: "empty parts",
			addr: &imap.Address{
				PersonalName: "",
				MailboxName:  "",
				HostName:     "",
			},
			want: "@",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAddress(tt.addr)
			if got != tt.want {
				t.Errorf("formatAddress(%+v) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

func TestFormatAddressList(t *testing.T) {
	tests := []struct {
		name  string
		addrs []*imap.Address
		want  string
	}{
		{
			name:  "nil slice",
			addrs: nil,
			want:  "",
		},
		{
			name:  "empty slice",
			addrs: []*imap.Address{},
			want:  "",
		},
		{
			name: "single address",
			addrs: []*imap.Address{
				{PersonalName: "Alice", MailboxName: "alice", HostName: "a.com"},
			},
			want: "Alice <alice@a.com>",
		},
		{
			name: "multiple addresses",
			addrs: []*imap.Address{
				{PersonalName: "Alice", MailboxName: "alice", HostName: "a.com"},
				{PersonalName: "", MailboxName: "bob", HostName: "b.org"},
			},
			want: "Alice <alice@a.com>, bob@b.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAddressList(tt.addrs)
			if got != tt.want {
				t.Errorf("formatAddressList(%+v) = %q, want %q", tt.addrs, got, tt.want)
			}
		})
	}
}