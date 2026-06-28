package cmd

import (
	"fmt"
	"time"

	"github.com/deepak-science/kestrel/internal/swoop"
)

// lastActivity returns the most recent timestamp from any action on a root.
func lastActivity(state *swoop.State, rootPath string) time.Time {
	if state == nil {
		return time.Time{}
	}
	e, ok := state.Entries[rootPath]
	if !ok {
		return time.Time{}
	}
	var latest time.Time
	for _, t := range []*time.Time{e.LastInit, e.LastPlan, e.LastApply} {
		if t != nil && t.After(latest) {
			latest = *t
		}
	}
	return latest
}

// lastActivityStr returns a human-readable string for the most recent action.
func lastActivityStr(state *swoop.State, rootPath string) string {
	if state == nil {
		return "-"
	}
	e, ok := state.Entries[rootPath]
	if !ok {
		return "-"
	}

	var latest time.Time
	var action string

	if e.LastApply != nil && e.LastApply.After(latest) {
		latest = *e.LastApply
		action = "apply"
	}
	if e.LastPlan != nil && e.LastPlan.After(latest) {
		latest = *e.LastPlan
		action = "plan"
		if e.PlanResult != "" {
			action = fmt.Sprintf("plan (%s)", e.PlanResult)
		}
	}
	if e.LastInit != nil && e.LastInit.After(latest) {
		latest = *e.LastInit
		action = "init"
	}

	if latest.IsZero() {
		return "-"
	}
	return fmt.Sprintf("%s %s", action, relativeTime(latest))
}

// relativeTime returns a human-friendly relative time string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
