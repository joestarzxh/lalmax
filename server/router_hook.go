package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/q191201771/lal/pkg/base"
)

func (s *LalMaxServer) initHookRouter(router *gin.Engine, handlers ...gin.HandlerFunc) {
	hook := router.Group("/api/hook", handlers...)
	hook.GET("/recent", s.hookRecentHandler)
	hook.GET("/stream", s.hookStreamHandler)
}

func (s *LalMaxServer) hookRecentHandler(c *gin.Context) {
	var out struct {
		base.ApiRespBasic
		Data struct {
			Events []HookEvent `json:"events"`
		} `json:"data"`
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	eventNames := ParseHookEventNames(c.Query("events"))
	if eventName := c.Query("event"); eventName != "" {
		eventNames = append(eventNames, eventName)
	}
	filter := NewHookEventFilter(c.Query("app_name"), c.Query("stream_name"), c.Query("session_id"), eventNames)

	out.ErrorCode = base.ErrorCodeSucc
	out.Desp = base.DespSucc
	out.Data.Events = s.notifyHub.RecentFiltered(limit, filter)
	c.JSON(http.StatusOK, out)
}

func (s *LalMaxServer) hookStreamHandler(c *gin.Context) {
	if s.notifyHub == nil {
		c.JSON(http.StatusOK, base.ApiRespBasic{
			ErrorCode: http.StatusInternalServerError,
			Desp:      "hook hub not initialized",
		})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusOK, base.ApiRespBasic{
			ErrorCode: http.StatusInternalServerError,
			Desp:      "streaming unsupported",
		})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	_, ch, cancel := s.notifyHub.Subscribe(0)
	defer cancel()

	eventNames := ParseHookEventNames(c.Query("events"))
	if eventName := c.Query("event"); eventName != "" {
		eventNames = append(eventNames, eventName)
	}
	filter := NewHookEventFilter(c.Query("app_name"), c.Query("stream_name"), c.Query("session_id"), eventNames)

	history := s.notifyHub.RecentFiltered(20, filter)
	lastHistoryID := int64(0)
	for _, event := range history {
		if event.ID > lastHistoryID {
			lastHistoryID = event.ID
		}
		if err := writeHookEventSSE(c.Writer, event); err != nil {
			return
		}
		flusher.Flush()
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if event.ID <= lastHistoryID {
				continue
			}
			if !filter.Match(event) {
				continue
			}
			if err := writeHookEventSSE(c.Writer, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeHookEventSSE(w http.ResponseWriter, event HookEvent) error {
	if _, err := fmt.Fprintf(w, "id: %d\n", event.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", event.Payload); err != nil {
		return err
	}
	return nil
}
