package hook

import (
	"sync"
	"time"

	"github.com/q191201771/lalmax/fmp4/hls"

	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/naza/pkg/nazalog"
)

var _ base.ISession = (*consumerInfo)(nil)

type IHookSessionSubscriber interface {
	OnMsg(msg base.RtmpMsg)
	OnStop()
}

type IHookSessionReplaySubscriber interface {
	OnReplayStart()
	OnReplayStop()
}

type HookSession struct {
	uniqueKey   string
	streamName  string
	consumers   sync.Map
	hlssvr      *hls.HlsServer
	gopCache    *GopCache
	gopCacheMux sync.RWMutex
	msgMux      sync.Mutex
	hasVideo    bool
}

type consumerInfo struct {
	subscriber   IHookSessionSubscriber
	hasSendVideo bool
	replayCache  bool
	writeMux     sync.Mutex

	base.StatSession
}

// AppName implements base.ISession.
func (c *consumerInfo) AppName() string {
	return c.SessionId
}

// GetStat implements base.ISession.
func (c *consumerInfo) GetStat() base.StatSession {
	return c.StatSession
}

// IsAlive implements base.ISession.
func (c *consumerInfo) IsAlive() (readAlive bool, writeAlive bool) {
	return true, true
}

// RawQuery implements base.ISession.
func (c *consumerInfo) RawQuery() string {
	return ""
}

// StreamName implements base.ISession.
func (c *consumerInfo) StreamName() string {
	return c.SessionId
}

// UniqueKey implements base.ISession.
func (c *consumerInfo) UniqueKey() string {
	return c.SessionId
}

// UpdateStat implements base.ISession.
func (c *consumerInfo) UpdateStat(intervalSec uint32) {
}

// Url implements base.ISession.
func (*consumerInfo) Url() string {
	return ""
}

func NewHookSession(uniqueKey, streamName string, hlssvr *hls.HlsServer, gopNum, singleGopMaxFrameNum int) *HookSession {
	s := &HookSession{
		uniqueKey:  uniqueKey,
		streamName: streamName,
		hlssvr:     hlssvr,
		gopCache:   NewGopCache(gopNum, singleGopMaxFrameNum),
	}

	if s.hlssvr != nil {
		s.hlssvr.NewHlsSession(streamName)
	}

	nazalog.Infof("create hook session, uniqueKey:%s, streamName:%s", uniqueKey, streamName)

	GetHookSessionManagerInstance().SetHookSession(streamName, s)
	return s
}

func (session *HookSession) OnMsg(msg base.RtmpMsg) {
	if session.hlssvr != nil {
		session.hlssvr.OnMsg(session.streamName, msg)
	}

	session.msgMux.Lock()
	hasVideo := session.hasVideo
	consumers := make([]*consumerInfo, 0)
	session.consumers.Range(func(key, value interface{}) bool {
		if c, ok := value.(*consumerInfo); ok {
			consumers = append(consumers, c)
		}
		return true
	})

	if !session.hasVideo && msg.IsVideoKeyNalu() {
		session.hasVideo = true
	}

	session.gopCacheMux.Lock()
	session.gopCache.Feed(msg)
	session.gopCacheMux.Unlock()
	session.msgMux.Unlock()

	for _, c := range consumers {
		session.handleConsumerMsg(c, msg, hasVideo)
	}
}

func (session *HookSession) OnStop() {
	if session.hlssvr != nil {
		session.hlssvr.OnStop(session.streamName)
	}

	nazalog.Debugf("OnStop, uniqueKey:%s, streamName:%s", session.uniqueKey, session.streamName)
	session.consumers.Range(func(key, value interface{}) bool {
		c := value.(*consumerInfo)
		if c.subscriber != nil {
			c.subscriber.OnStop()
		}
		return true
	})

	GetHookSessionManagerInstance().RemoveHookSession(session.streamName)
}

func (session *HookSession) AddConsumer(consumerId string, subscriber IHookSessionSubscriber) {
	session.AddConsumerWithReplay(consumerId, subscriber, true)
}

func (session *HookSession) AddConsumerWithReplay(consumerId string, subscriber IHookSessionSubscriber, replayCache bool) {
	info := &consumerInfo{
		subscriber:  subscriber,
		replayCache: replayCache,
		StatSession: base.StatSession{
			SessionId: consumerId,
			StartTime: time.Now().Format(time.DateTime),
			// Protocol: , TODO: (xugo)需要传递更多的参数来填充数据
		},
	}

	nazalog.Info("AddConsumer, consumerId:", consumerId)
	if replayCache {
		info.writeMux.Lock()
	}
	var replayMsgs []base.RtmpMsg

	session.msgMux.Lock()
	session.consumers.Store(consumerId, info)
	if replayCache {
		replayMsgs = session.getGopReplayMessages()
	}
	session.msgMux.Unlock()

	if replayCache {
		session.replayGopMessagesLocked(info, replayMsgs)
		info.writeMux.Unlock()
	}
}

func (session *HookSession) GetAllConsumer() []base.StatSub {
	out := make([]base.StatSub, 0, 10)
	session.consumers.Range(func(key, value any) bool {
		v, ok := value.(*consumerInfo)
		if ok {
			// TODO: (xugo)先简单实现，此处需要优化数据准确性
			out = append(out, base.Session2StatSub(v))
		}
		return true
	})
	return out
}

func (session *HookSession) RemoveConsumer(consumerId string) {
	_, ok := session.consumers.Load(consumerId)
	if ok {
		nazalog.Info("RemoveConsumer, consumerId:", consumerId)
		session.consumers.Delete(consumerId)
	}
}

func (session *HookSession) GetVideoSeqHeaderMsg() *base.RtmpMsg {
	session.gopCacheMux.RLock()
	defer session.gopCacheMux.RUnlock()
	if session.gopCache.videoheader == nil {
		return nil
	}
	m := session.gopCache.videoheader.Clone()
	return &m
}

func (session *HookSession) GetAudioSeqHeaderMsg() *base.RtmpMsg {
	session.gopCacheMux.RLock()
	defer session.gopCacheMux.RUnlock()
	if session.gopCache.audioheader == nil {
		return nil
	}
	m := session.gopCache.audioheader.Clone()
	return &m
}

func (session *HookSession) handleConsumerMsg(c *consumerInfo, msg base.RtmpMsg, hasVideo bool) {
	if c == nil {
		return
	}

	c.writeMux.Lock()
	defer c.writeMux.Unlock()

	if c.subscriber == nil {
		return
	}

	if msg.Header.MsgTypeId == base.RtmpTypeIdVideo {
		if !c.hasSendVideo {
			if !msg.IsVideoKeyNalu() {
				return
			}
			if v := session.GetVideoSeqHeaderMsg(); v != nil {
				c.subscriber.OnMsg(*v)
			}
			if v := session.GetAudioSeqHeaderMsg(); v != nil && v.IsAacSeqHeader() {
				c.subscriber.OnMsg(*v)
			}
			c.hasSendVideo = true
		}

		c.subscriber.OnMsg(msg)
	} else if msg.Header.MsgTypeId == base.RtmpTypeIdAudio {
		if !hasVideo || c.hasSendVideo {
			c.subscriber.OnMsg(msg)
		}
	}
}

func (session *HookSession) replayGopMessagesLocked(c *consumerInfo, msgs []base.RtmpMsg) {
	if c == nil || c.subscriber == nil || c.hasSendVideo || !c.replayCache {
		return
	}

	if len(msgs) == 0 {
		return
	}

	if replaySubscriber, ok := c.subscriber.(IHookSessionReplaySubscriber); ok {
		replaySubscriber.OnReplayStart()
		defer replaySubscriber.OnReplayStop()
	}

	for _, msg := range msgs {
		c.subscriber.OnMsg(msg)
	}
	c.hasSendVideo = true
}

func (session *HookSession) getGopReplayMessages() []base.RtmpMsg {
	session.gopCacheMux.RLock()
	defer session.gopCacheMux.RUnlock()

	gopCount := session.gopCache.GetGopCount()
	if gopCount == 0 {
		return nil
	}

	msgs := make([]base.RtmpMsg, 0, gopCount)
	if v := session.gopCache.videoheader; v != nil {
		msgs = append(msgs, v.Clone())
	}
	if v := session.gopCache.audioheader; v != nil && v.IsAacSeqHeader() {
		msgs = append(msgs, v.Clone())
	}
	for i := 0; i < gopCount; i++ {
		for _, item := range session.gopCache.GetGopDataAt(i) {
			msgs = append(msgs, item.Clone())
		}
	}

	return msgs
}
