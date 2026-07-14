package mail

import (
	"github.com/egorefimow/emailer/internal/config"
)

// DefaultLabels are the built-in classification labels that ship with the
// emailer. Custom labels from configuration are added on top.
var DefaultLabels = []string{"Useful", "ToDelete", "Ads"}

// labelSet returns the full set of valid classification labels including
// built-in defaults and any custom labels from configuration.
func labelSet(cfg config.LabelsConfig) map[string]bool {
	s := make(map[string]bool, len(DefaultLabels)+len(cfg.Custom))
	for _, l := range DefaultLabels {
		s[l] = true
	}
	for _, l := range cfg.Custom {
		s[l] = true
	}
	return s
}

// ClassificationToFlag maps a Classification to an IMAP keyword Flag.
//
// Built-in labels (Useful, ToDelete, Ads) and any custom labels from
// configuration are recognized. If the classification label is unknown,
// the returned Flag has an empty Keyword, indicating no flag should be
// applied.
func ClassificationToFlag(c Classification, cfg config.LabelsConfig) Flag {
	valid := labelSet(cfg)
	if !valid[c.Label] {
		return Flag{Key: c.Key, Keyword: ""}
	}
	return Flag{Key: c.Key, Keyword: c.Label}
}

// ValidLabel reports whether label is a known classification label
// (built-in or custom).
func ValidLabel(label string, cfg config.LabelsConfig) bool {
	return labelSet(cfg)[label]
}