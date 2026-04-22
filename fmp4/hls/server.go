package hls

import (
	"sync"
	"time"

	config "github.com/q191201771/lalmax/config"

	"github.com/gin-gonic/gin"
	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/naza/pkg/nazalog"
)

type HlsServer struct {
	sessions        sync.Map
	conf            config.Fmp4HlsConfig
	invalidSessions sync.Map
}

func NewHlsServer(conf config.Fmp4HlsConfig) *HlsServer {
	svr := &HlsServer{
		conf: conf,
	}

	go svr.cleanInvalidSession()

	return svr
}

func (s *HlsServer) NewHlsSession(streamName string) {
	s.NewHlsSessionWithAppName("", streamName)
}

func (s *HlsServer) NewHlsSessionWithAppName(appName, streamName string) {
	nazalog.Infof("new hls session, appName:%s, streamName:%s", appName, streamName)
	session := NewHlsSessionWithAppName(appName, streamName, s.conf)
	s.sessions.Store(hlsSessionKey(appName, streamName), session)
}

func (s *HlsServer) OnMsg(streamName string, msg base.RtmpMsg) {
	s.OnMsgWithAppName("", streamName, msg)
}

func (s *HlsServer) OnMsgWithAppName(appName, streamName string, msg base.RtmpMsg) {
	value, ok := s.sessions.Load(hlsSessionKey(appName, streamName))
	if ok {
		session := value.(*HlsSession)
		session.OnMsg(msg)
	}
}

func (s *HlsServer) OnStop(streamName string) {
	s.OnStopWithAppName("", streamName)
}

func (s *HlsServer) OnStopWithAppName(appName, streamName string) {
	key := hlsSessionKey(appName, streamName)
	value, ok := s.sessions.Load(key)
	if ok {
		session := value.(*HlsSession)
		s.invalidSessions.Store(session.SessionId, session)
		s.sessions.Delete(key)
	}
}

func (s *HlsServer) HandleRequest(ctx *gin.Context) {
	streamName := ctx.Param("streamid")
	appName := ctx.Query("app_name")
	if session, ok := s.getSession(appName, streamName); ok {
		session.HandleRequest(ctx)
	}
}

func (s *HlsServer) getSession(appName, streamName string) (*HlsSession, bool) {
	value, ok := s.sessions.Load(hlsSessionKey(appName, streamName))
	if ok {
		return value.(*HlsSession), true
	}

	if appName != "" {
		return nil, false
	}

	var found *HlsSession
	matchCount := 0
	s.sessions.Range(func(_, value interface{}) bool {
		session := value.(*HlsSession)
		if session.streamName != streamName {
			return true
		}
		found = session
		matchCount++
		return matchCount <= 1
	})
	if matchCount != 1 {
		return nil, false
	}
	return found, true
}

type sessionKey struct {
	appName    string
	streamName string
}

func hlsSessionKey(appName, streamName string) sessionKey {
	return sessionKey{
		appName:    appName,
		streamName: streamName,
	}
}

func (s *HlsServer) cleanInvalidSession() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.invalidSessions.Range(func(k, v interface{}) bool {
			session := v.(*HlsSession)
			nazalog.Info("clean invalid session, streamName:", session.streamName, " sessionId:", k)
			session.OnStop()
			s.invalidSessions.Delete(k)
			return true
		})
	}
}
