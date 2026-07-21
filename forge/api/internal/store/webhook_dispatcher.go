package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// DispatchWebhookEvent is retained for legacy call sites. It synchronously
// persists the event; delivery is always performed by the durable worker.
func (s *Store) DispatchWebhookEvent(event string, payload map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.EnqueueWebhookEvent(ctx, event, payload)
}

// eventMatchesSubscription supports exact, global, and prefix wildcard event subscriptions.
func eventMatchesSubscription(event string, subs []string) bool {
	for _, sub := range subs {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}
		if sub == "*" || sub == event {
			return true
		}
		if strings.HasSuffix(sub, ":*") && strings.HasPrefix(event, strings.TrimSuffix(sub, ":*")+":") {
			return true
		}
		if strings.HasSuffix(sub, "*") && strings.HasPrefix(event, strings.TrimSuffix(sub, "*")) {
			return true
		}
	}
	return false
}

func WrapDiscordEmbed(wh Webhook, event string, raw []byte) []byte {
	var base map[string]any
	_ = json.Unmarshal(raw, &base)
	title := event
	if value, ok := base["resource_type"].(string); ok && value != "" {
		title = value + " · " + event
	}
	description := ""
	if value, ok := base["name"].(string); ok {
		description = value
	} else if value, ok := base["resource_id"].(string); ok {
		description = value
	}
	color := 0x95a5a6
	switch {
	case strings.HasSuffix(event, ":created"), strings.HasSuffix(event, ":started"), strings.HasSuffix(event, ":installed"):
		color = 0x2ecc71
	case strings.HasSuffix(event, ":deleted"), strings.HasSuffix(event, ":suspended"), strings.HasSuffix(event, ":crashed"):
		color = 0xe74c3c
	case strings.HasSuffix(event, ":stopped"), strings.HasSuffix(event, ":transferred"):
		color = 0xe67e22
	case strings.HasSuffix(event, ":updated"), strings.HasSuffix(event, ":restored"), strings.HasSuffix(event, ":reinstalled"):
		color = 0x3498db
	}
	username := wh.DiscordUsername
	if username == "" {
		username = "Forge"
	}
	payload := map[string]any{"username": username, "embeds": []any{map[string]any{"title": title, "description": description, "color": color, "timestamp": base["timestamp"]}}}
	if wh.DiscordAvatarURL != "" {
		payload["avatar_url"] = wh.DiscordAvatarURL
	}
	if wh.DiscordContent != "" {
		payload["content"] = wh.DiscordContent
	}
	body, _ := json.Marshal(payload)
	return body
}
