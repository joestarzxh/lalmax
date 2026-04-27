package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/q191201771/lal/pkg/base"
	maxlogic "github.com/q191201771/lalmax/logic"
)

func (s *LalMaxServer) initStatRouter(router *gin.Engine, handlers ...gin.HandlerFunc) {
	stat := router.Group("/api/stat", handlers...)
	stat.GET("/group", s.statGroupHandler)
	stat.GET("/all_group", s.statAllGroupHandler)
	stat.GET("/lal_info", s.statLalInfoHandler)
}

func (s *LalMaxServer) statGroupHandler(c *gin.Context) {
	var v ApiStatGroupResp
	streamName := c.Query("stream_name")
	if streamName == "" {
		v.ErrorCode = base.ErrorCodeParamMissing
		v.Desp = base.DespParamMissing
		c.JSON(http.StatusOK, v)
		return
	}
	appName := c.Query("app_name")
	view := s.stats.FindGroupView(s.lalsvr.StatAllGroup(), maxlogic.NewStreamKey(appName, streamName))
	if view == nil {
		v.ErrorCode = base.ErrorCodeGroupNotFound
		v.Desp = base.DespGroupNotFound
		c.JSON(http.StatusOK, v)
		return
	}
	group := newLalmaxStatGroup(*view)
	v.Data = &group
	v.ErrorCode = base.ErrorCodeSucc
	v.Desp = base.DespSucc
	c.JSON(http.StatusOK, v)
}

func (s *LalMaxServer) statAllGroupHandler(c *gin.Context) {
	var out ApiStatAllGroupResp
	out.ErrorCode = base.ErrorCodeSucc
	out.Desp = base.DespSucc
	out.Data.Groups = newLalmaxStatGroups(s.stats.BuildGroupsView(s.lalsvr.StatAllGroup()))
	c.JSON(http.StatusOK, out)
}

func (s *LalMaxServer) statLalInfoHandler(c *gin.Context) {
	var v base.ApiStatLalInfoResp
	v.ErrorCode = base.ErrorCodeSucc
	v.Desp = base.DespSucc
	v.Data = s.lalsvr.StatLalInfo()
	c.JSON(http.StatusOK, v)
}
