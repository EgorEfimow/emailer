package mail

import (
	"context"
	"testing"

	"github.com/emersion/go-imap"
)

func TestChunkUIDs(t *testing.T) {
	make := func(n int) []uint32 {
		u := make([]uint32, n)
		for i := range u {
			u[i] = uint32(i + 1)
		}
		return u
	}

	tests := []struct {
		name     string
		uids     []uint32
		size     int
		wantN    int
		wantLast int
	}{
		{name: "empty, size 10", uids: nil, size: 10, wantN: 1, wantLast: 0},
		{name: "exact multiple", uids: make(20), size: 10, wantN: 2, wantLast: 10},
		{name: "remainder", uids: make(25), size: 10, wantN: 3, wantLast: 5},
		{name: "size larger than input", uids: make(3), size: 10, wantN: 1, wantLast: 3},
		{name: "non-positive size", uids: make(25), size: 0, wantN: 1, wantLast: 25},
		{name: "single uid", uids: make(1), size: 10, wantN: 1, wantLast: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := chunkUIDs(tt.uids, tt.size)
			if len(chunks) != tt.wantN {
				t.Fatalf("chunkUIDs len = %d, want %d", len(chunks), tt.wantN)
			}
			if len(chunks[len(chunks)-1]) != tt.wantLast {
				t.Errorf("last chunk size = %d, want %d", len(chunks[len(chunks)-1]), tt.wantLast)
			}
		})
	}
}

func TestFetchHeaders_NotConnected(t *testing.T) {
	c := newTestClient(t)

	_, err := c.fetchHeaders(context.Background(), []uint32{1, 2, 3}, 0)
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