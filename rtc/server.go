package rtc

import (
	"fmt"
	"net"
	"net/http"
	"time"

	config "github.com/q191201771/lalmax/config"
	maxlogic "github.com/q191201771/lalmax/logic"

	"github.com/gin-gonic/gin"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
	"github.com/q191201771/lal/pkg/logic"
	"github.com/q191201771/naza/pkg/nazalog"
)

// StreamNotFoundFn 流不存在时的回调，触发 on_stream_not_found 通知上层拉流
type StreamNotFoundFn func(app, stream, schema string)

type RtcServer struct {
	config           config.RtcConfig
	lalServer        logic.ILalServer
	udpMux           ice.UDPMux
	tcpMux           ice.TCPMux
	streamNotFoundFn StreamNotFoundFn
}

// SetStreamNotFoundFn 注入流不存在回调
func (s *RtcServer) SetStreamNotFoundFn(fn StreamNotFoundFn) {
	s.streamNotFoundFn = fn
}

// waitStreamReady 触发 on_stream_not_found 后轮询等待流就绪
// 为什么：WebRTC 播放请求先于 GB28181 设备推流到达，需通知上层拉流后等待
func (s *RtcServer) waitStreamReady(appName, streamid, schema string) bool {
	key := maxlogic.NewStreamKey(appName, streamid)
	if ok, _ := maxlogic.GetGroupManagerInstance().GetGroup(key); ok {
		return true
	}

	if s.streamNotFoundFn != nil {
		nazalog.Infof("stream not found, triggering on_stream_not_found. app=%s, stream=%s", appName, streamid)
		s.streamNotFoundFn(appName, streamid, schema)
	}

	ok, _ := maxlogic.GetGroupManagerInstance().WaitGroup(key, 500*time.Millisecond, 5*time.Second)
	return ok
}

func NewRtcServer(config config.RtcConfig, lal logic.ILalServer) (*RtcServer, error) {
	var udpMux ice.UDPMux
	var tcpMux ice.TCPMux

	if config.ICEUDPMuxPort != 0 {
		var udplistener *net.UDPConn

		udplistener, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: config.ICEUDPMuxPort,
		})

		if err != nil {
			nazalog.Error(err)
			return nil, err
		}
		nazalog.Infof("webrtc ice udp listen. port=%d", config.ICEUDPMuxPort)
		udpMux = webrtc.NewICEUDPMux(nil, udplistener)
	}
	if config.WriteChanSize == 0 {
		config.WriteChanSize = 1024
	}
	if config.ICETCPMuxPort != 0 {
		var tcplistener *net.TCPListener

		tcplistener, err := net.ListenTCP("tcp", &net.TCPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: config.ICETCPMuxPort,
		})

		if err != nil {
			nazalog.Error(err)
			return nil, err
		}
		nazalog.Infof("webrtc ice tcp listen. port=%d", config.ICETCPMuxPort)
		tcpMux = webrtc.NewICETCPMux(nil, tcplistener, 20)
	}

	svr := &RtcServer{
		config:    config,
		lalServer: lal,
		udpMux:    udpMux,
		tcpMux:    tcpMux,
	}

	return svr, nil
}

func (s *RtcServer) HandleWHIP(c *gin.Context) {
	streamid := c.Request.URL.Query().Get("streamid")
	if streamid == "" {
		c.Status(http.StatusMethodNotAllowed)
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		nazalog.Error(err)
		c.Status(http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		nazalog.Error("invalid body")
		c.Status(http.StatusNoContent)
		return
	}

	pc, err := newPeerConnection(s.config.ICEHostNATToIPs, s.udpMux, s.tcpMux)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	whipsession := NewWhipSession(streamid, pc, s.lalServer)
	if whipsession == nil {
		c.Status(http.StatusInternalServerError)
		pc.Close()
		return
	}

	c.Header("Location", fmt.Sprintf("whip/%s", whipsession.subscriberId))

	sdp := whipsession.GetAnswerSDP(string(body))
	if sdp == "" {
		c.Status(http.StatusInternalServerError)
		whipsession.Close()
		return
	}

	go whipsession.Run()

	c.Data(http.StatusCreated, "application/sdp", []byte(sdp))
}
func (s *RtcServer) HandleJessibuca(c *gin.Context) {
	streamid := c.Param("streamid")
	if streamid == "" {
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	appName := c.Query("app_name")

	body, err := c.GetRawData()
	if err != nil {
		nazalog.Error(err)
		c.Status(http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		nazalog.Error("invalid body")
		c.Status(http.StatusNoContent)
		return
	}

	if !s.waitStreamReady(appName, streamid, "rtsp") {
		nazalog.Errorf("stream not ready after waiting. app=%s, stream=%s", appName, streamid)
		c.Status(http.StatusNotFound)
		return
	}

	pc, err := newPeerConnection(s.config.ICEHostNATToIPs, s.udpMux, s.tcpMux)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	jessibucaSession := NewJessibucaSession(appName, streamid, s.config.WriteChanSize, pc, s.lalServer)
	if jessibucaSession == nil {
		c.Status(http.StatusInternalServerError)
		pc.Close()
		return
	}

	c.Header("Location", fmt.Sprintf("jessibucaflv/%s", jessibucaSession.subscriberId))

	sdp := jessibucaSession.GetAnswerSDP(string(body))
	if sdp == "" {
		c.Status(http.StatusInternalServerError)
		jessibucaSession.Close()
		return
	}

	go jessibucaSession.Run()

	c.Data(http.StatusCreated, "application/sdp", []byte(sdp))
}
func (s *RtcServer) HandleWHEP(c *gin.Context) {
	streamid := c.Request.URL.Query().Get("streamid")
	if streamid == "" {
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	appName := c.Request.URL.Query().Get("app_name")

	body, err := c.GetRawData()
	if err != nil {
		nazalog.Error(err)
		c.Status(http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		nazalog.Error("invalid body")
		c.Status(http.StatusNoContent)
		return
	}

	if !s.waitStreamReady(appName, streamid, "rtsp") {
		nazalog.Errorf("stream not ready after waiting. app=%s, stream=%s", appName, streamid)
		c.Status(http.StatusNotFound)
		return
	}

	pc, err := newPeerConnection(s.config.ICEHostNATToIPs, s.udpMux, s.tcpMux)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	whepsession := NewWhepSession(appName, streamid, s.config.WriteChanSize, pc, s.lalServer)
	if whepsession == nil {
		c.Status(http.StatusInternalServerError)
		pc.Close()
		return
	}

	c.Header("Location", fmt.Sprintf("whep/%s", whepsession.subscriberId))

	sdp := whepsession.GetAnswerSDP(string(body))
	if sdp == "" {
		c.Status(http.StatusInternalServerError)
		whepsession.Close()
		return
	}

	go whepsession.Run()

	c.Data(http.StatusCreated, "application/sdp", []byte(sdp))
}

// HandleZlmWebrtcPlay ZLM 兼容 WebRTC 播放，返回 SDP answer
// 为什么独立方法：ZLM 信令格式为 JSON {"code":0,"sdp":"..."}，与 WHEP 纯 SDP 不同
func (s *RtcServer) HandleZlmWebrtcPlay(app, stream, offer string) (string, error) {
	if !s.waitStreamReady(app, stream, "rtsp") {
		return "", fmt.Errorf("stream not found: %s/%s", app, stream)
	}

	pc, err := newPeerConnection(s.config.ICEHostNATToIPs, s.udpMux, s.tcpMux)
	if err != nil {
		return "", fmt.Errorf("create peer connection: %w", err)
	}

	session := NewWhepSession(app, stream, s.config.WriteChanSize, pc, s.lalServer)
	if session == nil {
		pc.Close()
		return "", fmt.Errorf("create session failed: %s/%s", app, stream)
	}

	sdp := session.GetAnswerSDP(offer)
	if sdp == "" {
		session.Close()
		return "", fmt.Errorf("generate answer sdp failed")
	}

	go session.Run()
	return sdp, nil
}
