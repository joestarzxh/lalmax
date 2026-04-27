package httpfmp4

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/q191201771/lalmax/fmp4/muxer"
	maxlogic "github.com/q191201771/lalmax/logic"

	"github.com/gofrs/uuid"
	"github.com/q191201771/naza/pkg/connection"

	"github.com/gin-gonic/gin"
	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/naza/pkg/nazalog"
)

var ErrWriteChanFull = errors.New("Fmp4  Session write channel full")

var (
	readBufSize = 4096 //  session connection读缓冲的大小
	wChanSize   = 256  //  session 发送数据时，channel 的大小
)

type HttpFmp4Session struct {
	appName      string
	streamid     string
	group        *maxlogic.Group
	subscriberId string

	rtmp2Fmp4Remuxer *muxer.Rtmp2Fmp4Remuxer
	w                gin.ResponseWriter
	conn             connection.Connection
	disposeOnce      sync.Once
	log              nazalog.Logger
}

func NewHttpFmp4Session(appName, streamid string) *HttpFmp4Session {

	streamid = strings.TrimSuffix(streamid, ".mp4")
	u, _ := uuid.NewV4()

	session := &HttpFmp4Session{
		appName:      appName,
		streamid:     streamid,
		subscriberId: u.String(),
		log:          nazalog.WithPrefix(u.String()),
	}

	session.rtmp2Fmp4Remuxer = muxer.NewRtmp2Fmp4Remuxer(session).WithLog(session.log)

	session.log.Infof("create http fmp4 session, appName:%s, streamid:%s", appName, streamid)

	return session
}
func (session *HttpFmp4Session) OnInitFmp4(init []byte) {
	session.conn.Write(init)
}

func (session *HttpFmp4Session) OnFmp4Packets(currentPart *muxer.MuxerPart, lastSampleDuration time.Duration, end bool, isVideo bool) {
	if currentPart != nil {
		if err := currentPart.Encode(lastSampleDuration, end); err == nil {
			session.conn.Write(currentPart.Bytes())
		}
	}
}

func (session *HttpFmp4Session) Dispose() error {
	return session.dispose()
}
func (session *HttpFmp4Session) dispose() error {
	var retErr error
	session.disposeOnce.Do(func() {
		session.OnStop()
		if session.conn == nil {
			retErr = base.ErrSessionNotStarted
			return
		}
		retErr = session.conn.Close()
	})
	return retErr
}
func (session *HttpFmp4Session) handleSession(c *gin.Context) {
	ok, group := maxlogic.GetGroupManagerInstance().GetGroup(maxlogic.NewStreamKey(session.appName, session.streamid))
	if !ok {
		nazalog.Errorf("stream is not found, appName:%s, streamid:%s", session.appName, session.streamid)
		c.Status(http.StatusNotFound)
		return
	}

	session.group = group
	session.w = c.Writer

	c.Header("Content-Type", "video/mp4")
	c.Header("Connection", "close")
	c.Header("Expires", "-1")
	h, ok := session.w.(http.Hijacker)
	if !ok {
		nazalog.Error("gin response does not implement http.Hijacker")
		return
	}

	conn, bio, err := h.Hijack()
	if err != nil {
		nazalog.Errorf("hijack failed. err=%+v", err)
		return
	}
	if bio.Reader.Buffered() != 0 || bio.Writer.Buffered() != 0 {
		nazalog.Errorf("hijack but buffer not empty. rb=%d, wb=%d", bio.Reader.Buffered(), bio.Writer.Buffered())
	}
	session.conn = connection.New(conn, func(option *connection.Option) {
		option.ReadBufSize = readBufSize
		option.WriteChanSize = wChanSize
	})
	if err = session.writeHttpHeader(session.w.Header()); err != nil {
		nazalog.Errorf("session writeHttpHeader. err=%+v", err)
		return
	}
	session.group.AddSubscriber(maxlogic.SubscriberInfo{
		SubscriberID: session.subscriberId,
		Protocol:     maxlogic.SubscriberProtocolHTTPFMP4,
	}, session)

	go func() {
		readBuf := make([]byte, 1024)
		_, err = session.conn.Read(readBuf)
		session.dispose()
	}()

}

func (session *HttpFmp4Session) writeHttpHeader(header http.Header) error {
	p := make([]byte, 0, 1024)
	p = append(p, []byte("HTTP/1.1 200 OK\r\n")...)
	for k, vs := range header {
		for _, v := range vs {
			p = append(p, k...)
			p = append(p, ": "...)
			for i := 0; i < len(v); i++ {
				b := v[i]
				if b <= 31 {
					// prevent response splitting.
					b = ' '
				}
				p = append(p, b)
			}
			p = append(p, "\r\n"...)
		}
	}
	p = append(p, "\r\n"...)

	return session.write(p)
}
func (session *HttpFmp4Session) write(buf []byte) (err error) {
	if session.conn != nil {
		_, err = session.conn.Write(buf)
	}
	return err
}
func (session *HttpFmp4Session) OnMsg(msg base.RtmpMsg) {
	if session.rtmp2Fmp4Remuxer != nil {
		session.rtmp2Fmp4Remuxer.FeedRtmpMessage(msg)
	}
}

func (session *HttpFmp4Session) OnStop() {
	if session.group != nil {
		session.group.RemoveSubscriber(session.subscriberId)
	}
}

func (session *HttpFmp4Session) GetSubscriberStat() maxlogic.SubscriberStat {
	if session == nil || session.conn == nil {
		return maxlogic.SubscriberStat{}
	}

	connStat := session.conn.GetStat()
	stat := maxlogic.SubscriberStat{
		ReadBytesSum:  connStat.ReadBytesSum,
		WroteBytesSum: connStat.WroteBytesSum,
	}
	if remoteAddr := session.conn.RemoteAddr(); remoteAddr != nil {
		stat.RemoteAddr = remoteAddr.String()
	}
	return stat
}
