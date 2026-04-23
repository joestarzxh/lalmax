package logic

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/q191201771/lalmax/fmp4/hls"

	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/naza/pkg/nazalog"
)

var _ base.ISession = (*subscriberState)(nil)

const (
	SubscriberProtocolLalmax    = "LALMAX"
	SubscriberProtocolWHEP      = "WHEP"
	SubscriberProtocolJessibuca = "JESSIBUCA"
	SubscriberProtocolHTTPFMP4  = "HTTP-FMP4"
	SubscriberProtocolSRT       = "SRT"
)

type Subscriber interface {
	OnMsg(msg base.RtmpMsg)
	OnStop()
}

// 可选接口：订阅者需要区分 GOP 回放和实时帧时实现。
type ReplaySubscriber interface {
	OnReplayStart()
	OnReplayStop()
}

type SubscriberInfo struct {
	SubscriberID string
	Protocol     string
	RemoteAddr   string
}

// Group 只维护 lalmax 侧订阅者和回放缓存，推流状态仍以 lal 为准。
type Group struct {
	uniqueKey    string
	key          StreamKey
	consumers    sync.Map
	hlssvr       *hls.HlsServer
	manager      *ComplexGroupManager
	gopCache     *GopCache
	gopCacheMux  sync.RWMutex
	lifecycleMux sync.RWMutex
	stopOnce     sync.Once
	msgMux       sync.Mutex
	hasVideo     bool
	closed       atomic.Bool
}

type subscriberState struct {
	key          StreamKey
	subscriber   Subscriber
	hasSendVideo bool
	replayCache  bool
	writeMux     sync.Mutex
	stopped      atomic.Bool

	base.StatSession
}

func (s *subscriberState) AppName() string {
	return s.key.AppName
}

func (s *subscriberState) GetStat() base.StatSession {
	return s.StatSession
}

func (s *subscriberState) IsAlive() (readAlive bool, writeAlive bool) {
	return true, true
}

func (s *subscriberState) RawQuery() string {
	return ""
}

func (s *subscriberState) StreamName() string {
	return s.key.StreamName
}

func (s *subscriberState) UniqueKey() string {
	return s.SessionId
}

func (s *subscriberState) UpdateStat(intervalSec uint32) {
}

func (s *subscriberState) Url() string {
	return s.key.String()
}

func newGroup(manager *ComplexGroupManager, uniqueKey string, key StreamKey, hlssvr *hls.HlsServer, gopNum, singleGopMaxFrameNum int) *Group {
	group := &Group{
		uniqueKey: uniqueKey,
		key:       key,
		hlssvr:    hlssvr,
		manager:   manager,
		gopCache:  NewGopCache(gopNum, singleGopMaxFrameNum),
	}

	nazalog.Infof("create group, uniqueKey:%s, streamKey:%s", uniqueKey, key.String())

	return group
}

func (group *Group) initHlsSession() {
	if group != nil && group.hlssvr != nil {
		group.hlssvr.NewHlsSessionWithAppName(group.key.AppName, group.key.StreamName)
	}
}

func (group *Group) waitLifecycleIdle() {
	if group == nil {
		return
	}

	group.lifecycleMux.RLock()
	group.lifecycleMux.RUnlock()
}

func (group *Group) Key() StreamKey {
	return group.key
}

func (group *Group) UniqueKey() string {
	return group.uniqueKey
}

func (group *Group) OnMsg(msg base.RtmpMsg) {
	group.lifecycleMux.RLock()
	if group.closed.Load() {
		group.lifecycleMux.RUnlock()
		return
	}
	defer group.lifecycleMux.RUnlock()

	if group.hlssvr != nil {
		group.hlssvr.OnMsgWithAppName(group.key.AppName, group.key.StreamName, msg)
	}

	group.msgMux.Lock()
	hasVideo := group.hasVideo
	consumers := make([]*subscriberState, 0)
	group.consumers.Range(func(key, value interface{}) bool {
		if c, ok := value.(*subscriberState); ok {
			consumers = append(consumers, c)
		}
		return true
	})

	if !group.hasVideo && msg.IsVideoKeyNalu() {
		group.hasVideo = true
	}

	group.gopCacheMux.Lock()
	group.gopCache.Feed(msg)
	group.gopCacheMux.Unlock()
	group.msgMux.Unlock()

	for _, c := range consumers {
		group.handleSubscriberMsg(c, msg, hasVideo)
	}
}

func (group *Group) OnStop() {
	group.stopOnce.Do(func() {
		group.lifecycleMux.Lock()
		group.closed.Store(true)

		if group.hlssvr != nil {
			group.hlssvr.OnStopWithAppName(group.key.AppName, group.key.StreamName)
		}

		consumers := make([]*subscriberState, 0)
		group.consumers.Range(func(key, value interface{}) bool {
			c, ok := value.(*subscriberState)
			if ok {
				consumers = append(consumers, c)
			}
			group.consumers.Delete(key)
			return true
		})
		group.lifecycleMux.Unlock()

		nazalog.Debugf("OnStop, uniqueKey:%s, streamKey:%s", group.uniqueKey, group.key.String())
		for _, c := range consumers {
			c.stopWithNotify()
		}

		if group.manager != nil {
			group.manager.RemoveGroupIfMatch(group.key, group)
		}
	})
}

func (group *Group) AddSubscriber(info SubscriberInfo, subscriber Subscriber) {
	group.AddSubscriberWithReplay(info, subscriber, true)
}

func (group *Group) AddSubscriberWithReplay(info SubscriberInfo, subscriber Subscriber, replayCache bool) {
	if info.SubscriberID == "" {
		nazalog.Warn("AddSubscriber skipped, subscriber id is empty")
		return
	}
	if info.Protocol == "" {
		info.Protocol = SubscriberProtocolLalmax
	}

	group.lifecycleMux.RLock()
	if group.closed.Load() {
		group.lifecycleMux.RUnlock()
		nazalog.Warnf("AddSubscriber skipped, group is closed, streamKey:%s, subscriberId:%s", group.key.String(), info.SubscriberID)
		return
	}
	defer group.lifecycleMux.RUnlock()

	state := &subscriberState{
		key:         group.key,
		subscriber:  subscriber,
		replayCache: replayCache,
		StatSession: base.StatSession{
			SessionId:  info.SubscriberID,
			Protocol:   info.Protocol,
			BaseType:   base.SessionBaseTypeSubStr,
			RemoteAddr: info.RemoteAddr,
			StartTime:  time.Now().Format(time.DateTime),
		},
	}

	nazalog.Infof("AddSubscriber, streamKey:%s, subscriberId:%s, protocol:%s", group.key.String(), info.SubscriberID, info.Protocol)
	if replayCache {
		// 保证该订阅者先收到缓存 GOP，再收到实时帧。
		state.writeMux.Lock()
	}
	var replayMsgs []base.RtmpMsg

	group.msgMux.Lock()
	if _, loaded := group.consumers.Load(info.SubscriberID); loaded {
		group.msgMux.Unlock()
		if replayCache {
			state.writeMux.Unlock()
		}
		nazalog.Warnf("AddSubscriber skipped, subscriber already exists, streamKey:%s, subscriberId:%s", group.key.String(), info.SubscriberID)
		return
	}
	group.consumers.Store(info.SubscriberID, state)
	if replayCache {
		replayMsgs = group.getGopReplayMessages()
	}
	group.msgMux.Unlock()

	if replayCache {
		group.replayGopMessagesLocked(state, replayMsgs)
		state.writeMux.Unlock()
	}
}

func (group *Group) AddConsumer(consumerID string, subscriber Subscriber) {
	group.AddSubscriber(SubscriberInfo{SubscriberID: consumerID}, subscriber)
}

func (group *Group) AddConsumerWithReplay(consumerID string, subscriber Subscriber, replayCache bool) {
	group.AddSubscriberWithReplay(SubscriberInfo{SubscriberID: consumerID}, subscriber, replayCache)
}

func (group *Group) StatSubscribers() []base.StatSub {
	out := make([]base.StatSub, 0, 10)
	group.consumers.Range(func(key, value any) bool {
		v, ok := value.(*subscriberState)
		if ok {
			out = append(out, base.Session2StatSub(v))
		}
		return true
	})
	return out
}

func (group *Group) GetAllConsumer() []base.StatSub {
	return group.StatSubscribers()
}

func (group *Group) RemoveSubscriber(subscriberID string) {
	value, ok := group.consumers.LoadAndDelete(subscriberID)
	if ok {
		nazalog.Infof("RemoveSubscriber, streamKey:%s, subscriberId:%s", group.key.String(), subscriberID)
		if c, ok := value.(*subscriberState); ok {
			c.stopWithoutNotify()
		}
	}
}

func (group *Group) RemoveConsumer(consumerID string) {
	group.RemoveSubscriber(consumerID)
}

func (group *Group) GetVideoSeqHeaderMsg() *base.RtmpMsg {
	group.gopCacheMux.RLock()
	defer group.gopCacheMux.RUnlock()
	if group.gopCache.videoheader == nil {
		return nil
	}
	m := group.gopCache.videoheader.Clone()
	return &m
}

func (group *Group) GetAudioSeqHeaderMsg() *base.RtmpMsg {
	group.gopCacheMux.RLock()
	defer group.gopCacheMux.RUnlock()
	if group.gopCache.audioheader == nil {
		return nil
	}
	m := group.gopCache.audioheader.Clone()
	return &m
}

func (group *Group) handleSubscriberMsg(c *subscriberState, msg base.RtmpMsg, hasVideo bool) {
	if c == nil {
		return
	}

	c.writeMux.Lock()
	defer c.writeMux.Unlock()

	if c.stopped.Load() || c.subscriber == nil {
		return
	}

	if msg.Header.MsgTypeId == base.RtmpTypeIdVideo {
		if !c.hasSendVideo {
			if !msg.IsVideoKeyNalu() {
				return
			}
			if v := group.GetVideoSeqHeaderMsg(); v != nil {
				if !c.deliverMsg(*v) {
					return
				}
			}
			if v := group.GetAudioSeqHeaderMsg(); v != nil && v.IsAacSeqHeader() {
				if !c.deliverMsg(*v) {
					return
				}
			}
			c.hasSendVideo = true
		}

		c.deliverMsg(msg)
	} else if msg.Header.MsgTypeId == base.RtmpTypeIdAudio {
		if !hasVideo || c.hasSendVideo {
			c.deliverMsg(msg)
		}
	}
}

func (group *Group) replayGopMessagesLocked(c *subscriberState, msgs []base.RtmpMsg) {
	if c == nil || c.subscriber == nil || c.stopped.Load() || c.hasSendVideo || !c.replayCache {
		return
	}

	if len(msgs) == 0 {
		return
	}

	if replaySubscriber, ok := c.subscriber.(ReplaySubscriber); ok {
		replaySubscriber.OnReplayStart()
		defer replaySubscriber.OnReplayStop()
	}

	for _, msg := range msgs {
		if !c.deliverMsg(msg) {
			return
		}
	}
	c.hasSendVideo = true
}

func (s *subscriberState) deliverMsg(msg base.RtmpMsg) bool {
	if s == nil || s.stopped.Load() || s.subscriber == nil {
		return false
	}

	s.subscriber.OnMsg(msg)
	return !s.stopped.Load() && s.subscriber != nil
}

func (s *subscriberState) stopWithNotify() {
	if s == nil {
		return
	}

	s.writeMux.Lock()
	defer s.writeMux.Unlock()

	if s.stopped.Swap(true) {
		return
	}
	if s.subscriber != nil {
		s.subscriber.OnStop()
		s.subscriber = nil
	}
}

func (s *subscriberState) stopWithoutNotify() {
	if s == nil {
		return
	}

	// 不能在这里获取 writeMux：部分订阅者会在 OnMsg 调用栈内主动移除自己。
	// 只标记停止，避免后续投递；订阅者对象随 state 一起释放。
	s.stopped.Store(true)
}

func (group *Group) getGopReplayMessages() []base.RtmpMsg {
	group.gopCacheMux.RLock()
	defer group.gopCacheMux.RUnlock()

	gopCount := group.gopCache.GetGopCount()
	if gopCount == 0 {
		return nil
	}

	msgs := make([]base.RtmpMsg, 0, gopCount)
	if v := group.gopCache.videoheader; v != nil {
		msgs = append(msgs, v.Clone())
	}
	if v := group.gopCache.audioheader; v != nil && v.IsAacSeqHeader() {
		msgs = append(msgs, v.Clone())
	}
	for i := 0; i < gopCount; i++ {
		for _, item := range group.gopCache.GetGopDataAt(i) {
			msgs = append(msgs, item.Clone())
		}
	}

	return msgs
}
