package mediaserver

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/q191201771/lal/pkg/logic"
	"github.com/q191201771/naza/pkg/nazalog"
)

const defaultReadTimeout = 10 * time.Second

type IGbObserver interface {
	CheckSsrc(ssrc uint32) (*MediaInfo, bool)
	GetMediaInfoByKey(key string) (*MediaInfo, bool)
	NotifyClose(streamName string)
	OnRtpPacket(streamName string, mediaKey string)
}

type GB28181MediaServer struct {
	listenPort int
	lalServer  logic.ILalServer

	listener net.Listener

	disposeOnce          sync.Once
	disposed             atomic.Bool
	observer             IGbObserver
	mediaKey             string
	preferMediaKeyLookup bool
	readTimeout          time.Duration

	conns sync.Map //增加链接对象，目前只适用于多端口
}

func NewGB28181MediaServer(listenPort int, mediaKey string, observer IGbObserver, lal logic.ILalServer) *GB28181MediaServer {
	return &GB28181MediaServer{
		listenPort:  listenPort,
		lalServer:   lal,
		observer:    observer,
		mediaKey:    mediaKey,
		readTimeout: defaultReadTimeout,
	}
}

func (s *GB28181MediaServer) WithPreferMediaKeyLookup(prefer bool) *GB28181MediaServer {
	s.preferMediaKeyLookup = prefer
	return s
}

func (s *GB28181MediaServer) WithReadTimeout(timeout time.Duration) *GB28181MediaServer {
	s.readTimeout = timeout
	return s
}

func (s *GB28181MediaServer) GetListenerPort() uint16 {
	return uint16(s.listenPort)
}
func (s *GB28181MediaServer) Start(listener net.Listener) (err error) {
	s.listener = listener
	if listener != nil {
		go func(listener net.Listener) {
			for {
				if s.disposed.Load() {
					return
				}
				conn, err := listener.Accept()
				if err != nil {
					var ne net.Error
					if ok := errors.As(err, &ne); ok && ne.Timeout() {
						nazalog.Error("Accept failed: timeout error, retrying...")
						time.Sleep(time.Second / 20)
						continue
					} else {
						break
					}
				}
				if conn == nil {
					continue
				}
				if s.disposed.Load() {
					conn.Close()
					return
				}

				c := NewConn(conn, s.observer, s.lalServer)
				c.SetKey(s.mediaKey)
				c.SetMediaServer(s)
				c.SetPreferMediaKeyLookup(s.preferMediaKeyLookup)
				c.SetReadTimeout(s.readTimeout)
				s.conns.Store(c, c)
				go func() {
					c.Serve()
					s.conns.Delete(c)
					s.conns.Delete(c.streamName)
				}()
			}
		}(listener)
	}
	return
}
func (s *GB28181MediaServer) CloseConn(streamName string) {
	if v, ok := s.conns.Load(streamName); ok {
		conn := v.(*Conn)
		conn.Close()
	}
}
func (s *GB28181MediaServer) Dispose() {
	s.disposeOnce.Do(func() {
		s.disposed.Store(true)
		s.conns.Range(func(_, value any) bool {
			conn := value.(*Conn)
			conn.Close()
			return true
		})
		if s.listener != nil {
			s.listener.Close()
			s.listener = nil
		}
	})
}
