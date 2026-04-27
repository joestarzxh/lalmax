package server

import (
	"strings"

	maxlogic "github.com/q191201771/lalmax/logic"
)

type HookEventFilter struct {
	EventNames map[string]struct{}
	AppName    string
	StreamName string
	SessionID  string
}

func NewHookEventFilter(appName, streamName, sessionID string, eventNames []string) HookEventFilter {
	filter := HookEventFilter{
		AppName:    appName,
		StreamName: streamName,
		SessionID:  sessionID,
	}

	if len(eventNames) != 0 {
		filter.EventNames = make(map[string]struct{}, len(eventNames))
		for _, eventName := range eventNames {
			if eventName == "" {
				continue
			}
			filter.EventNames[eventName] = struct{}{}
		}
	}

	return filter
}

func ParseHookEventNames(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func (f HookEventFilter) Match(event HookEvent) bool {
	if len(f.EventNames) != 0 {
		if _, ok := f.EventNames[event.Event]; !ok {
			return false
		}
	}

	if f.SessionID != "" && event.sessionID != f.SessionID {
		return false
	}

	if f.AppName == "" && f.StreamName == "" {
		return true
	}

	if event.streamName != "" || event.appName != "" {
		return matchStreamKey(maxlogic.NewStreamKey(event.appName, event.streamName), f.AppName, f.StreamName)
	}

	for _, key := range event.groupKeys {
		if matchStreamKey(key, f.AppName, f.StreamName) {
			return true
		}
	}

	return false
}

func matchStreamKey(key maxlogic.StreamKey, appName, streamName string) bool {
	if streamName != "" && key.StreamName != streamName {
		return false
	}

	if appName != "" && key.AppName != appName {
		return false
	}

	if streamName == "" && appName != "" && key.AppName == "" {
		return false
	}

	return key.StreamName != "" || key.AppName != ""
}
