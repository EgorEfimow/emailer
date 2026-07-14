package mail

import (
	"testing"

	"github.com/egorefimow/emailer/internal/config"
)

func TestClassificationToFlag_BuiltinLabels(t *testing.T) {
	key := MessageKey{AccountLabel: "work", UID: 1}
	cfg := config.LabelsConfig{Custom: nil}

	tests := []struct {
		name    string
		label   string
		wantKw  string
		wantKey MessageKey
	}{
		{
			name:    "Useful label",
			label:   "Useful",
			wantKw:  "Useful",
			wantKey: key,
		},
		{
			name:    "ToDelete label",
			label:   "ToDelete",
			wantKw:  "ToDelete",
			wantKey: key,
		},
		{
			name:    "Ads label",
			label:   "Ads",
			wantKw:  "Ads",
			wantKey: key,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Classification{Key: key, Label: tt.label}
			got := ClassificationToFlag(c, cfg)
			if got.Keyword != tt.wantKw {
				t.Errorf("Flag.Keyword = %q, want %q", got.Keyword, tt.wantKw)
			}
			if got.Key != tt.wantKey {
				t.Errorf("Flag.Key = %+v, want %+v", got.Key, tt.wantKey)
			}
		})
	}
}

func TestClassificationToFlag_UnknownLabel(t *testing.T) {
	key := MessageKey{AccountLabel: "work", UID: 1}
	cfg := config.LabelsConfig{Custom: nil}

	c := Classification{Key: key, Label: "Spam"}
	got := ClassificationToFlag(c, cfg)

	if got.Keyword != "" {
		t.Errorf("unknown label: Flag.Keyword = %q, want empty", got.Keyword)
	}
	// Key should still be preserved so the caller can correlate.
	if got.Key != key {
		t.Errorf("unknown label: Flag.Key = %+v, want %+v", got.Key, key)
	}
}

func TestClassificationToFlag_CustomLabels(t *testing.T) {
	key := MessageKey{AccountLabel: "personal", UID: 42}
	cfg := config.LabelsConfig{Custom: []string{"FollowUp", "Archive"}}

	tests := []struct {
		name   string
		label  string
		wantKw string
	}{
		{
			name:   "custom FollowUp",
			label:  "FollowUp",
			wantKw: "FollowUp",
		},
		{
			name:   "custom Archive",
			label:  "Archive",
			wantKw: "Archive",
		},
		{
			name:   "built-in still works with custom config",
			label:  "Useful",
			wantKw: "Useful",
		},
		{
			name:   "label not in custom or built-in",
			label:  "Unknown",
			wantKw: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Classification{Key: key, Label: tt.label}
			got := ClassificationToFlag(c, cfg)
			if got.Keyword != tt.wantKw {
				t.Errorf("Flag.Keyword = %q, want %q", got.Keyword, tt.wantKw)
			}
		})
	}
}

func TestClassificationToFlag_KeyPreservation(t *testing.T) {
	// Ensure the MessageKey is faithfully passed through to the Flag.
	keys := []MessageKey{
		{AccountLabel: "work", UID: 1},
		{AccountLabel: "personal", UID: 999},
		{AccountLabel: "archive", UID: 0},
	}

	cfg := config.LabelsConfig{}

	for _, key := range keys {
		t.Run(key.Key(), func(t *testing.T) {
			c := Classification{Key: key, Label: "Useful"}
			got := ClassificationToFlag(c, cfg)
			if got.Key != key {
				t.Errorf("Flag.Key = %+v, want %+v", got.Key, key)
			}
		})
	}
}

func TestValidLabel(t *testing.T) {
	tests := []struct {
		name  string
		label string
		cfg   config.LabelsConfig
		want  bool
	}{
		{
			name:  "built-in Useful",
			label: "Useful",
			cfg:   config.LabelsConfig{},
			want:  true,
		},
		{
			name:  "built-in ToDelete",
			label: "ToDelete",
			cfg:   config.LabelsConfig{},
			want:  true,
		},
		{
			name:  "built-in Ads",
			label: "Ads",
			cfg:   config.LabelsConfig{},
			want:  true,
		},
		{
			name:  "unknown label",
			label: "Spam",
			cfg:   config.LabelsConfig{},
			want:  false,
		},
		{
			name:  "custom label",
			label: "FollowUp",
			cfg:   config.LabelsConfig{Custom: []string{"FollowUp"}},
			want:  true,
		},
		{
			name:  "custom label not in list",
			label: "FollowUp",
			cfg:   config.LabelsConfig{Custom: []string{"Archive"}},
			want:  false,
		},
		{
			name:  "empty string",
			label: "",
			cfg:   config.LabelsConfig{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidLabel(tt.label, tt.cfg)
			if got != tt.want {
				t.Errorf("ValidLabel(%q) = %v, want %v", tt.label, got, tt.want)
			}
		})
	}
}