package httpfmp4

import (
	"github.com/gin-gonic/gin"
)

type HttpFmp4Server struct {
}

func NewHttpFmp4Server() *HttpFmp4Server {
	svr := &HttpFmp4Server{}

	return svr
}

func (s *HttpFmp4Server) HandleRequest(c *gin.Context) {
	streamid := c.Param("streamid")
	appName := c.Query("app_name")

	session := NewHttpFmp4Session(appName, streamid)
	session.handleSession(c)
}
