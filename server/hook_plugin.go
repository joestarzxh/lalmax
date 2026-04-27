package server

import (
	"fmt"
	"sync"
)

const defaultHookPluginBufferSize = 64

type HookPlugin interface {
	Name() string
	OnHookEvent(event HookEvent) error
}

type HookPluginOptions struct {
	BufferSize int
	Filter     HookEventFilter
}

type hookPluginEntry struct {
	plugin HookPlugin
	filter HookEventFilter
	queue  chan HookEvent
}

func (h *HttpNotify) RegisterPlugin(plugin HookPlugin, options HookPluginOptions) (func(), error) {
	if h == nil {
		return nil, fmt.Errorf("hook hub is nil")
	}
	if plugin == nil {
		return nil, fmt.Errorf("hook plugin is nil")
	}
	if plugin.Name() == "" {
		return nil, fmt.Errorf("hook plugin name is empty")
	}

	bufferSize := options.BufferSize
	if bufferSize <= 0 {
		bufferSize = defaultHookPluginBufferSize
	}

	entry := &hookPluginEntry{
		plugin: plugin,
		filter: options.Filter,
		queue:  make(chan HookEvent, bufferSize),
	}

	h.pluginMux.Lock()
	if _, exists := h.plugins[plugin.Name()]; exists {
		h.pluginMux.Unlock()
		return nil, fmt.Errorf("hook plugin already exists: %s", plugin.Name())
	}
	h.plugins[plugin.Name()] = entry
	h.pluginMux.Unlock()

	go h.runPlugin(entry)

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.unregisterPlugin(plugin.Name())
		})
	}

	return cancel, nil
}

func (h *HttpNotify) runPlugin(entry *hookPluginEntry) {
	for event := range entry.queue {
		if err := entry.plugin.OnHookEvent(event); err != nil {
			Log.Errorf("hook plugin handle error. plugin=%s, event=%s, err=%+v", entry.plugin.Name(), event.Event, err)
		}
	}
}

func (h *HttpNotify) dispatchPlugins(event HookEvent) {
	h.pluginMux.RLock()
	defer h.pluginMux.RUnlock()

	for _, entry := range h.plugins {
		if !entry.filter.Match(event) {
			continue
		}

		select {
		case entry.queue <- event:
		default:
			Log.Warnf("hook plugin queue full. plugin=%s, event=%s", entry.plugin.Name(), event.Event)
		}
	}
}

func (h *HttpNotify) unregisterPlugin(name string) {
	h.pluginMux.Lock()
	defer h.pluginMux.Unlock()

	entry, ok := h.plugins[name]
	if !ok {
		return
	}

	delete(h.plugins, name)
	close(entry.queue)
}
