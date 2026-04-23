package server

import (
	"encoding/json"
	"io"
	"net/http"

	maxlogic "github.com/q191201771/lalmax/logic"

	"github.com/gin-gonic/gin"
	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/lal/pkg/logic"
	"github.com/q191201771/naza/pkg/nazahttp"
	"github.com/q191201771/naza/pkg/nazajson"
	"github.com/q191201771/naza/pkg/nazalog"
)

func (s *LalMaxServer) InitRouter(router *gin.Engine) {
	if router == nil {
		return
	}
	router.Use(s.Cors())

	rtc := router.Group("/webrtc")
	// whip
	rtc.POST("/whip", s.HandleWHIP)
	rtc.OPTIONS("/whip", s.HandleWHIP)
	rtc.DELETE("/whip", s.HandleWHIP)
	// whep
	rtc.POST("/whep", s.HandleWHEP)
	rtc.OPTIONS("/whep", s.HandleWHEP)
	rtc.DELETE("/whep", s.HandleWHEP)
	// Jessibuca flv封装play
	rtc.POST("/play/live/:streamid", s.HandleJessibuca)
	rtc.DELETE("/play/live/:streamid", s.HandleJessibuca)

	// http-fmp4
	router.GET("/live/m4s/:streamid", s.HandleHttpFmp4)

	// hls-fmp4/llhls
	router.GET("/live/hls/:streamid/:type", s.HandleHls)

	auth := Authentication(s.conf.HttpConfig.CtrlAuthWhitelist.Secrets, s.conf.HttpConfig.CtrlAuthWhitelist.IPs)
	// stat
	stat := router.Group("/api/stat", auth)
	stat.GET("/group", s.statGroupHandler)
	stat.GET("/all_group", s.statAllGroupHandler)
	stat.GET("/lal_info", s.statLalInfoHandler)

	// ctrl
	ctrl := router.Group("/api/ctrl", auth)
	ctrl.POST("/start_relay_pull", s.ctrlStartRelayPullHandler)
	ctrl.POST("/stop_relay_pull", s.ctrlStopRelayPullHandler)
	ctrl.POST("/kick_session", s.ctrlKickSessionHandler)
	ctrl.POST("/start_rtp_pub", s.ctrlStartRtpPubHandler)
	ctrl.POST("/stop_rtp_pub", s.ctrlStopRtpPubHandler)
}

func (s *LalMaxServer) HandleWHIP(c *gin.Context) {
	switch c.Request.Method {
	case "POST":
		if s.rtcsvr != nil {
			s.rtcsvr.HandleWHIP(c)
		}
	case "DELETE":
		// TODO 实现DELETE
		c.Status(http.StatusOK)
	}
}

func (s *LalMaxServer) HandleWHEP(c *gin.Context) {
	switch c.Request.Method {
	case "POST":
		if s.rtcsvr != nil {
			s.rtcsvr.HandleWHEP(c)
		}
	case "DELETE":
		// TODO 实现DELETE
		c.Status(http.StatusOK)
	}
}

func (s *LalMaxServer) HandleJessibuca(c *gin.Context) {
	switch c.Request.Method {
	case "POST":
		if s.rtcsvr != nil {
			s.rtcsvr.HandleJessibuca(c)
		}
	case "DELETE":
		// TODO 实现DELETE
		c.Status(http.StatusOK)
	}
}

func (s *LalMaxServer) HandleHls(c *gin.Context) {
	if s.hlssvr != nil {
		s.hlssvr.HandleRequest(c)
	} else {
		nazalog.Error("hls is disable")
		c.Status(http.StatusNotFound)
	}
}

func (s *LalMaxServer) HandleHttpFmp4(c *gin.Context) {
	if s.httpfmp4svr != nil {
		s.httpfmp4svr.HandleRequest(c)
	} else {
		nazalog.Error("http-fmp4 is disable")
		c.Status(http.StatusNotFound)
	}
}

func (s *LalMaxServer) statGroupHandler(c *gin.Context) {
	var v base.ApiStatGroupResp
	streamName := c.Query("stream_name")
	if streamName == "" {
		v.ErrorCode = base.ErrorCodeParamMissing
		v.Desp = base.DespParamMissing
		c.JSON(http.StatusOK, v)
		return
	}
	appName := c.Query("app_name")
	v.Data = s.lalsvr.StatGroup(streamName)
	if v.Data == nil {
		v.ErrorCode = base.ErrorCodeGroupNotFound
		v.Desp = base.DespGroupNotFound
		c.JSON(http.StatusOK, v)
		return
	}
	exist, session := maxlogic.GetGroupManagerInstance().GetGroup(maxlogic.NewStreamKey(appName, streamName))
	if exist {
		v.Data.StatSubs = append(v.Data.StatSubs, session.StatSubscribers()...)
	}
	v.ErrorCode = base.ErrorCodeSucc
	v.Desp = base.DespSucc
	c.JSON(http.StatusOK, v)
}

func (s *LalMaxServer) statAllGroupHandler(c *gin.Context) {
	var out base.ApiStatAllGroupResp
	out.ErrorCode = base.ErrorCodeSucc
	out.Desp = base.DespSucc
	groups := s.lalsvr.StatAllGroup()
	for i, group := range groups {
		exist, session := maxlogic.GetGroupManagerInstance().GetGroup(maxlogic.NewStreamKey(group.AppName, group.StreamName))
		if exist {
			groups[i].StatSubs = append(groups[i].StatSubs, session.StatSubscribers()...)
		}
	}
	out.Data.Groups = groups
	c.JSON(http.StatusOK, out)
}

func (s *LalMaxServer) statLalInfoHandler(c *gin.Context) {
	var v base.ApiStatLalInfoResp
	v.ErrorCode = base.ErrorCodeSucc
	v.Desp = base.DespSucc
	v.Data = s.lalsvr.StatLalInfo()
	c.JSON(http.StatusOK, v)
}

func (s *LalMaxServer) ctrlStartRelayPullHandler(c *gin.Context) {
	var info base.ApiCtrlStartRelayPullReq
	var v base.ApiCtrlStartRelayPullResp
	j, err := unmarshalRequestJSONBody(c.Request, &info, "url")
	if err != nil {
		Log.Warnf("http api start pull error. err=%+v", err)
		v.ErrorCode = base.ErrorCodeParamMissing
		v.Desp = base.DespParamMissing
		c.JSON(http.StatusOK, v)
		return
	}

	if !j.Exist("pull_timeout_ms") {
		info.PullTimeoutMs = logic.DefaultApiCtrlStartRelayPullReqPullTimeoutMs
	}
	if !j.Exist("pull_retry_num") {
		info.PullRetryNum = base.PullRetryNumNever
	}
	if !j.Exist("auto_stop_pull_after_no_out_ms") {
		info.AutoStopPullAfterNoOutMs = base.AutoStopPullAfterNoOutMsNever
	}
	if !j.Exist("rtsp_mode") {
		info.RtspMode = base.RtspModeTcp
	}

	Log.Infof("http api start pull. req info=%+v", info)

	resp := s.lalsvr.CtrlStartRelayPull(info)
	c.JSON(http.StatusOK, resp)
}

func (s *LalMaxServer) ctrlStopRelayPullHandler(c *gin.Context) {
	var v base.ApiCtrlStopRelayPullResp
	streamName := c.Query("stream_name")
	if streamName == "" {
		v.ErrorCode = base.ErrorCodeParamMissing
		v.Desp = base.DespParamMissing
		c.JSON(http.StatusOK, v)
		return
	}

	Log.Infof("http api stop pull. stream_name=%s", streamName)

	resp := s.lalsvr.CtrlStopRelayPull(streamName)
	c.JSON(http.StatusOK, resp)
}

func (s *LalMaxServer) ctrlKickSessionHandler(c *gin.Context) {
	var v base.ApiCtrlKickSessionResp
	var info base.ApiCtrlKickSessionReq

	_, err := unmarshalRequestJSONBody(c.Request, &info, "stream_name", "session_id")
	if err != nil {
		Log.Warnf("http api kick session error. err=%+v", err)
		v.ErrorCode = base.ErrorCodeParamMissing
		v.Desp = base.DespParamMissing
		c.JSON(http.StatusOK, v)
		return
	}

	Log.Infof("http api kick session. req info=%+v", info)

	resp := s.lalsvr.CtrlKickSession(info)
	c.JSON(http.StatusOK, resp)
}

func (s *LalMaxServer) ctrlStartRtpPubHandler(c *gin.Context) {
	var v base.ApiCtrlStartRtpPubResp
	var info base.ApiCtrlStartRtpPubReq

	j, err := unmarshalRequestJSONBody(c.Request, &info, "stream_name")
	if err != nil {
		Log.Warnf("http api start rtp pub error. err=%+v", err)
		v.ErrorCode = base.ErrorCodeParamMissing
		v.Desp = base.DespParamMissing
		c.JSON(http.StatusOK, v)
		return
	}

	if !j.Exist("timeout_ms") {
		info.TimeoutMs = logic.DefaultApiCtrlStartRtpPubReqTimeoutMs
	}

	Log.Infof("http api start rtp pub. req info=%+v", info)

	resp := s.rtpPubMgr.Start(info)
	c.JSON(http.StatusOK, resp)
}

func (s *LalMaxServer) ctrlStopRtpPubHandler(c *gin.Context) {
	var v base.ApiCtrlStopRelayPullResp
	streamName := c.Query("stream_name")
	sessionID := c.Query("session_id")

	if streamName == "" && sessionID == "" {
		var info base.ApiCtrlKickSessionReq
		if _, err := unmarshalRequestJSONBody(c.Request, &info); err == nil {
			streamName = info.StreamName
			sessionID = info.SessionId
		}
	}

	if streamName == "" && sessionID == "" {
		v.ErrorCode = base.ErrorCodeParamMissing
		v.Desp = base.DespParamMissing
		c.JSON(http.StatusOK, v)
		return
	}

	Log.Infof("http api stop rtp pub. stream_name=%s, session_id=%s", streamName, sessionID)

	session, err := s.rtpPubMgr.Stop(streamName, sessionID)
	if err != nil {
		v.ErrorCode = base.ErrorCodeSessionNotFound
		v.Desp = err.Error()
		c.JSON(http.StatusOK, v)
		return
	}

	v.ErrorCode = base.ErrorCodeSucc
	v.Desp = base.DespSucc
	v.Data.SessionId = session.ID
	c.JSON(http.StatusOK, v)
}

func unmarshalRequestJSONBody(r *http.Request, info interface{}, keyFieldList ...string) (nazajson.Json, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nazajson.Json{}, err
	}

	j, err := nazajson.New(body)
	if err != nil {
		return j, err
	}
	for _, kf := range keyFieldList {
		if !j.Exist(kf) {
			return j, nazahttp.ErrParamMissing
		}
	}

	return j, json.Unmarshal(body, info)
}
