package components

import (
	"strings"

	"github.com/nkhine/gohtmxelm/demo/internal/presence"
)

func presenceClass(snapshot presence.Snapshot) string {
	switch snapshot.State {
	case presence.Online:
		return "presence-online"
	case presence.Idle:
		return "presence-idle"
	default:
		return "presence-offline"
	}
}

func presenceLabel(snapshot presence.Snapshot) string {
	switch snapshot.State {
	case presence.Online:
		if snapshot.Email != "" {
			return "Online as " + snapshot.Email
		}
		return "Online"
	case presence.Idle:
		if snapshot.Email != "" {
			return "Idle as " + snapshot.Email
		}
		return "Idle"
	default:
		return "Logged out"
	}
}

func presenceSignalState(snapshot presence.Snapshot) string {
	if snapshot.State == "" {
		return string(presence.Offline)
	}
	return string(snapshot.State)
}

func presenceSignalEmail(snapshot presence.Snapshot) string {
	return strings.TrimSpace(snapshot.Email)
}
