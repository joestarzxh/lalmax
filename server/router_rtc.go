package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *LalMaxServer) initRtcRouter(router *gin.Engine) {
	rtc := router.Group("/webrtc")
	rtc.POST("/whip", s.HandleWHIP)
	rtc.OPTIONS("/whip", s.HandleWHIP)
	rtc.DELETE("/whip", s.HandleWHIP)

	rtc.POST("/whep", s.HandleWHEP)
	rtc.OPTIONS("/whep", s.HandleWHEP)
	rtc.DELETE("/whep", s.HandleWHEP)

	rtc.POST("/play/live/:streamid", s.HandleJessibuca)
	rtc.DELETE("/play/live/:streamid", s.HandleJessibuca)
}

func (s *LalMaxServer) HandleWHIP(c *gin.Context) {
	switch c.Request.Method {
	case "POST":
		if s.rtcsvr != nil {
			s.rtcsvr.HandleWHIP(c)
		}
	case "DELETE":
		// TODO 实现 DELETE
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
		// TODO 实现 DELETE
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
		// TODO 实现 DELETE
		c.Status(http.StatusOK)
	}
}
