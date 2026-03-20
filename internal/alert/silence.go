package alert

import "time"

// Silence suppresses alerts matching a given rule name and/or label matchers
// for a specified duration.
type Silence struct {
	ID        string            `json:"id"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Rule      string            `json:"rule,omitempty"`       // if set, only silence alerts from this rule
	Matchers  map[string]string `json:"matchers,omitempty"`   // if set, all must match alert labels
	Comment   string            `json:"comment,omitempty"`
}

// Matches returns true if this silence applies to the given alert.
func (s Silence) Matches(a Alert) bool {
	if s.Rule != "" && s.Rule != a.Rule {
		return false
	}
	for k, v := range s.Matchers {
		if a.Labels[k] != v {
			return false
		}
	}
	return true
}

// IsExpired returns true if the silence has passed its expiration time.
func (s Silence) IsExpired(now time.Time) bool {
	return now.After(s.ExpiresAt)
}
