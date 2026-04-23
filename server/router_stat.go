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
