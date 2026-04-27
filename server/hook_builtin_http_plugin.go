package server

import "fmt"

type hookBuiltinHTTPPlugin struct {
	name string
	hub  *HttpNotify
}

func (p *hookBuiltinHTTPPlugin) Name() string {
	return p.name
}

func (p *hookBuiltinHTTPPlugin) OnHookEvent(event HookEvent) error {
	if p == nil || p.hub == nil {
		return nil
	}
	if !p.hub.cfg.Enable {
		return nil
	}

	switch event.Event {
	case HookEventServerStart:
		if p.hub.cfg.OnServerStart != "" {
			p.hub.asyncPost(p.hub.cfg.OnServerStart, event.Payload)
		}
	case HookEventUpdate:
		if p.hub.cfg.OnUpdate != "" {
			p.hub.notifyUpdateAsyncPost(p.hub.cfg.OnUpdate, event.Payload)
		}
	case HookEventGroupStart:
		if p.hub.cfg.OnGroupStart != "" {
			p.hub.asyncPost(p.hub.cfg.OnGroupStart, event.Payload)
		}
	case HookEventGroupStop:
		if p.hub.cfg.OnGroupStop != "" {
			p.hub.asyncPost(p.hub.cfg.OnGroupStop, event.Payload)
		}
	case HookEventStreamActive:
		if p.hub.cfg.OnStreamActive != "" {
			p.hub.asyncPost(p.hub.cfg.OnStreamActive, event.Payload)
		}
	case HookEventPubStart:
		if p.hub.cfg.OnPubStart != "" {
			p.hub.asyncPost(p.hub.cfg.OnPubStart, event.Payload)
		}
	case HookEventPubStop:
		if p.hub.cfg.OnPubStop != "" {
			p.hub.asyncPost(p.hub.cfg.OnPubStop, event.Payload)
		}
	case HookEventSubStart:
		if p.hub.cfg.OnSubStart != "" {
			p.hub.asyncPost(p.hub.cfg.OnSubStart, event.Payload)
		}
	case HookEventSubStop:
		if p.hub.cfg.OnSubStop != "" {
			p.hub.asyncPost(p.hub.cfg.OnSubStop, event.Payload)
		}
	case HookEventRelayPullStart:
		if p.hub.cfg.OnRelayPullStart != "" {
			p.hub.asyncPost(p.hub.cfg.OnRelayPullStart, event.Payload)
		}
	case HookEventRelayPullStop:
		if p.hub.cfg.OnRelayPullStop != "" {
			p.hub.asyncPost(p.hub.cfg.OnRelayPullStop, event.Payload)
		}
	case HookEventRtmpConnect:
		if p.hub.cfg.OnRtmpConnect != "" {
			p.hub.asyncPost(p.hub.cfg.OnRtmpConnect, event.Payload)
		}
	case HookEventHlsMakeTs:
		if p.hub.cfg.OnHlsMakeTs != "" {
			p.hub.asyncPost(p.hub.cfg.OnHlsMakeTs, event.Payload)
		}
	}

	return nil
}

func (h *HttpNotify) mustRegisterBuiltinHTTPPlugin() {
	if h == nil {
		return
	}

	_, err := h.RegisterPlugin(&hookBuiltinHTTPPlugin{
		name: "builtin-http-notify",
		hub:  h,
	}, HookPluginOptions{})
	if err != nil {
		panic(fmt.Sprintf("register builtin http hook plugin failed: %v", err))
	}
}
