// Copyright 2020, Chef.  All rights reserved.
// https://github.com/q191201771/lal
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	maxlogic "github.com/q191201771/lalmax/logic"

	config "github.com/q191201771/lalmax/config"

	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/naza/pkg/nazahttp"
	"github.com/q191201771/naza/pkg/nazalog"
)

// TODO(chef): refactor 配置参数供外部传入
// TODO(chef): refactor maxTaskLen修改为能表示是阻塞任务的意思
var (
	maxTaskLen       = 1024
	notifyTimeoutSec = 3
	hookHistorySize  = 256
	hookSubBufSize   = 64
)

var Log = nazalog.GetGlobalLogger()

type PostTask struct {
	url  string
	info interface{}
}

type HookGroupInfo struct {
	base.EventCommonInfo
	AppName    string `json:"app_name"`
	StreamName string `json:"stream_name"`
}

type HookEvent struct {
	ID        int64           `json:"id"`
	Event     string          `json:"event"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`

	sessionID  string
	streamName string
	appName    string
	groupKeys  []maxlogic.StreamKey
}

const (
	HookEventServerStart    = "on_server_start"
	HookEventUpdate         = "on_update"
	HookEventGroupStart     = "on_group_start"
	HookEventGroupStop      = "on_group_stop"
	HookEventStreamActive   = "on_stream_active"
	HookEventPubStart       = "on_pub_start"
	HookEventPubStop        = "on_pub_stop"
	HookEventSubStart       = "on_sub_start"
	HookEventSubStop        = "on_sub_stop"
	HookEventRelayPullStart = "on_relay_pull_start"
	HookEventRelayPullStop  = "on_relay_pull_stop"
	HookEventRtmpConnect    = "on_rtmp_connect"
	HookEventHlsMakeTs      = "on_hls_make_ts"
)

type HttpNotify struct {
	cfg config.HttpNotifyConfig

	serverId string
	stats    *maxlogic.StatAggregator

	taskQueue         chan PostTask
	notifyUpdateQueue chan PostTask
	client            *http.Client

	eventID     atomic.Int64
	subID       atomic.Int64
	historyMux  sync.RWMutex
	history     []HookEvent
	subscriberM sync.RWMutex
	subscribers map[int64]chan HookEvent
	pluginMux   sync.RWMutex
	plugins     map[string]*hookPluginEntry
}

func NewHttpNotify(cfg config.HttpNotifyConfig, serverId string) *HttpNotify {
	httpNotify := &HttpNotify{
		cfg:               cfg,
		serverId:          serverId,
		stats:             maxlogic.NewStatAggregator(maxlogic.GetGroupManagerInstance()),
		taskQueue:         make(chan PostTask, maxTaskLen),
		notifyUpdateQueue: make(chan PostTask, maxTaskLen),
		history:           make([]HookEvent, 0, hookHistorySize),
		subscribers:       make(map[int64]chan HookEvent),
		plugins:           make(map[string]*hookPluginEntry),
		client: &http.Client{
			Timeout: time.Duration(notifyTimeoutSec) * time.Second,
		},
	}
	httpNotify.mustRegisterBuiltinHTTPPlugin()
	go httpNotify.RunLoop()
	go httpNotify.NotifyUpdateRunLoop()

	return httpNotify
}

// TODO(chef): Dispose

// ---------------------------------------------------------------------------------------------------------------------

func (h *HttpNotify) NotifyServerStart(info base.LalInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventServerStart, info)
}

func (h *HttpNotify) NotifyUpdate(info base.UpdateInfo) {
	info.ServerId = h.serverId
	info.Groups = h.stats.MergeGroups(info.Groups)
	h.publish(HookEventUpdate, info)
}

func (h *HttpNotify) NotifyGroupStart(info HookGroupInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventGroupStart, info)
}

func (h *HttpNotify) NotifyGroupStop(info HookGroupInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventGroupStop, info)
}

func (h *HttpNotify) NotifyStreamActive(info HookGroupInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventStreamActive, info)
}

func (h *HttpNotify) NotifyPubStart(info base.PubStartInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventPubStart, info)
}

func (h *HttpNotify) NotifyPubStop(info base.PubStopInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventPubStop, info)
}

func (h *HttpNotify) NotifySubStart(info base.SubStartInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventSubStart, info)
}

func (h *HttpNotify) NotifySubStop(info base.SubStopInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventSubStop, info)
}

func (h *HttpNotify) NotifyPullStart(info base.PullStartInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventRelayPullStart, info)
}

func (h *HttpNotify) NotifyPullStop(info base.PullStopInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventRelayPullStop, info)
}

func (h *HttpNotify) NotifyRtmpConnect(info base.RtmpConnectInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventRtmpConnect, info)
}

func (h *HttpNotify) NotifyOnHlsMakeTs(info base.HlsMakeTsInfo) {
	info.ServerId = h.serverId
	h.publish(HookEventHlsMakeTs, info)
}

// ----- implement INotifyHandler interface ----------------------------------------------------------------------------

func (h *HttpNotify) OnServerStart(info base.LalInfo) {
	h.NotifyServerStart(info)
}

func (h *HttpNotify) OnUpdate(info base.UpdateInfo) {
	h.NotifyUpdate(info)
}

func (h *HttpNotify) OnGroupStart(info HookGroupInfo) {
	h.NotifyGroupStart(info)
}

func (h *HttpNotify) OnGroupStop(info HookGroupInfo) {
	h.NotifyGroupStop(info)
}

func (h *HttpNotify) OnStreamActive(info HookGroupInfo) {
	h.NotifyStreamActive(info)
}

func (h *HttpNotify) OnPubStart(info base.PubStartInfo) {
	h.NotifyPubStart(info)
}

func (h *HttpNotify) OnPubStop(info base.PubStopInfo) {
	h.NotifyPubStop(info)
}

func (h *HttpNotify) OnSubStart(info base.SubStartInfo) {
	h.NotifySubStart(info)
}

func (h *HttpNotify) OnSubStop(info base.SubStopInfo) {
	h.NotifySubStop(info)
}

func (h *HttpNotify) OnRelayPullStart(info base.PullStartInfo) {
	h.NotifyPullStart(info)
}

func (h *HttpNotify) OnRelayPullStop(info base.PullStopInfo) {
	h.NotifyPullStop(info)
}

func (h *HttpNotify) OnRtmpConnect(info base.RtmpConnectInfo) {
	h.NotifyRtmpConnect(info)
}

func (h *HttpNotify) OnHlsMakeTs(info base.HlsMakeTsInfo) {
	h.NotifyOnHlsMakeTs(info)
}

// ---------------------------------------------------------------------------------------------------------------------

func (h *HttpNotify) RunLoop() {
	for {
		select {
		case t := <-h.taskQueue:
			h.post(t.url, t.info)
		}
	}
}

func (h *HttpNotify) NotifyUpdateRunLoop() {
	for {
		select {
		case t := <-h.notifyUpdateQueue:
			h.post(t.url, t.info)
		}
	}
}

// ---------------------------------------------------------------------------------------------------------------------

func (h *HttpNotify) notifyUpdateAsyncPost(url string, info interface{}) {
	if !h.cfg.Enable || url == "" {
		return
	}

	select {
	case h.notifyUpdateQueue <- PostTask{url: url, info: info}:
		// noop
	default:
		Log.Error("http notify queue full.")
	}
}

func (h *HttpNotify) asyncPost(url string, info interface{}) {
	if !h.cfg.Enable || url == "" {
		return
	}

	select {
	case h.taskQueue <- PostTask{url: url, info: info}:
		// noop
	default:
		Log.Error("http notify queue full.")
	}
}

func (h *HttpNotify) post(url string, info interface{}) {
	switch v := info.(type) {
	case json.RawMessage:
		h.postRaw(url, v)
		return
	case []byte:
		h.postRaw(url, v)
		return
	}

	resp, err := nazahttp.PostJson(url, info, h.client)
	if err != nil {
		Log.Errorf("http notify post error. err=%+v, url=%s, info=%+v", err, url, info)
		return
	}
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func (h *HttpNotify) postRaw(url string, payload []byte) {
	if h == nil || url == "" || len(payload) == 0 {
		return
	}

	body := bytes.NewBuffer(payload)
	client := h.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Post(url, nazahttp.HeaderFieldContentType, body)
	if err != nil {
		Log.Errorf("http notify post raw payload error. err=%+v, url=%s, payload=%s", err, url, string(payload))
		return
	}
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func (h *HttpNotify) Recent(limit int) []HookEvent {
	h.historyMux.RLock()
	defer h.historyMux.RUnlock()

	if limit <= 0 || limit > len(h.history) {
		limit = len(h.history)
	}

	start := len(h.history) - limit
	out := make([]HookEvent, limit)
	copy(out, h.history[start:])
	return out
}

func (h *HttpNotify) RecentFiltered(limit int, filter HookEventFilter) []HookEvent {
	h.historyMux.RLock()
	defer h.historyMux.RUnlock()

	if limit <= 0 || limit > len(h.history) {
		limit = len(h.history)
	}

	out := make([]HookEvent, 0, limit)
	for i := len(h.history) - 1; i >= 0 && len(out) < limit; i-- {
		if !filter.Match(h.history[i]) {
			continue
		}
		out = append(out, h.history[i])
	}

	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (h *HttpNotify) Subscribe(buffer int) (int64, <-chan HookEvent, func()) {
	if buffer <= 0 {
		buffer = hookSubBufSize
	}

	id := h.subID.Add(1)
	ch := make(chan HookEvent, buffer)

	h.subscriberM.Lock()
	h.subscribers[id] = ch
	h.subscriberM.Unlock()

	cancel := func() {
		h.subscriberM.Lock()
		if sub, ok := h.subscribers[id]; ok {
			delete(h.subscribers, id)
			close(sub)
		}
		h.subscriberM.Unlock()
	}

	return id, ch, cancel
}

func (h *HttpNotify) publish(event string, info interface{}) {
	if h == nil {
		return
	}

	payload, err := json.Marshal(info)
	if err != nil {
		Log.Errorf("marshal hook event failed. event=%s, err=%+v", event, err)
		return
	}

	hookEvent := HookEvent{
		ID:        h.eventID.Add(1),
		Event:     event,
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Payload:   payload,
	}
	populateHookEventMeta(&hookEvent, info)

	h.historyMux.Lock()
	h.history = append(h.history, hookEvent)
	if len(h.history) > hookHistorySize {
		h.history = append([]HookEvent(nil), h.history[len(h.history)-hookHistorySize:]...)
	}
	h.historyMux.Unlock()

	h.dispatchPlugins(hookEvent)

	h.subscriberM.RLock()
	stale := make([]int64, 0)
	for id, ch := range h.subscribers {
		select {
		case ch <- hookEvent:
		default:
			stale = append(stale, id)
		}
	}
	h.subscriberM.RUnlock()

	if len(stale) == 0 {
		return
	}

	h.subscriberM.Lock()
	for _, id := range stale {
		if ch, ok := h.subscribers[id]; ok {
			delete(h.subscribers, id)
			close(ch)
		}
	}
	h.subscriberM.Unlock()
}

func populateHookEventMeta(event *HookEvent, info interface{}) {
	if event == nil || info == nil {
		return
	}

	switch v := info.(type) {
	case base.UpdateInfo:
		event.groupKeys = make([]maxlogic.StreamKey, 0, len(v.Groups))
		for _, group := range v.Groups {
			event.groupKeys = append(event.groupKeys, maxlogic.NewStreamKey(group.AppName, group.StreamName))
		}
	case HookGroupInfo:
		event.streamName = v.StreamName
		event.appName = v.AppName
	case base.PubStartInfo:
		populateHookSessionMeta(event, v.SessionEventCommonInfo)
	case base.PubStopInfo:
		populateHookSessionMeta(event, v.SessionEventCommonInfo)
	case base.SubStartInfo:
		populateHookSessionMeta(event, v.SessionEventCommonInfo)
	case base.SubStopInfo:
		populateHookSessionMeta(event, v.SessionEventCommonInfo)
	case base.PullStartInfo:
		populateHookSessionMeta(event, v.SessionEventCommonInfo)
	case base.PullStopInfo:
		populateHookSessionMeta(event, v.SessionEventCommonInfo)
	case base.RtmpConnectInfo:
		event.sessionID = v.SessionId
		event.appName = v.App
	case base.HlsMakeTsInfo:
		event.streamName = v.StreamName
	}
}

func populateHookSessionMeta(event *HookEvent, info base.SessionEventCommonInfo) {
	if event == nil {
		return
	}

	event.sessionID = info.SessionId
	event.streamName = info.StreamName
	event.appName = info.AppName
}
