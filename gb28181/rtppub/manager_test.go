package rtppub

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/lalmax/config"
)

func freeTCPPort(t *testing.T) uint16 {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	return uint16(listener.Addr().(*net.TCPAddr).Port)
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()

	return NewManager(nil, config.GB28181MediaConfig{})
}

func TestManagerStartStopBySessionID(t *testing.T) {
	manager := newTestManager(t)

	resp := manager.Start(base.ApiCtrlStartRtpPubReq{
		StreamName: "rtp-pub-start-stop",
		Port:       int(freeTCPPort(t)),
		TimeoutMs:  0,
		IsTcpFlag:  1,
	})
	if resp.ErrorCode != base.ErrorCodeSucc {
		t.Fatalf("start failed, code=%d desp=%s", resp.ErrorCode, resp.Desp)
	}
	if resp.Data.SessionId == "" || resp.Data.Port == 0 {
		t.Fatalf("unexpected start response: %+v", resp.Data)
	}

	session, err := manager.Stop("", resp.Data.SessionId)
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != resp.Data.SessionId {
		t.Fatalf("stopped session id = %s, want %s", session.ID, resp.Data.SessionId)
	}

	if _, err = manager.Stop("", resp.Data.SessionId); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("second stop err = %v, want %v", err, errSessionNotFound)
	}
}

func TestManagerRejectsDuplicateStream(t *testing.T) {
	manager := newTestManager(t)

	resp := manager.Start(base.ApiCtrlStartRtpPubReq{
		StreamName: "rtp-pub-duplicate",
		Port:       int(freeTCPPort(t)),
		TimeoutMs:  0,
		IsTcpFlag:  1,
	})
	if resp.ErrorCode != base.ErrorCodeSucc {
		t.Fatalf("start failed, code=%d desp=%s", resp.ErrorCode, resp.Desp)
	}
	defer manager.Stop(resp.Data.StreamName, "")

	duplicate := manager.Start(base.ApiCtrlStartRtpPubReq{
		StreamName: "rtp-pub-duplicate",
		Port:       int(freeTCPPort(t)),
		TimeoutMs:  0,
		IsTcpFlag:  1,
	})
	if duplicate.ErrorCode == base.ErrorCodeSucc {
		t.Fatalf("duplicate stream start unexpectedly succeeded: %+v", duplicate.Data)
	}
}

func TestManagerTimeoutRemovesIdleSession(t *testing.T) {
	manager := newTestManager(t)

	resp := manager.Start(base.ApiCtrlStartRtpPubReq{
		StreamName: "rtp-pub-timeout",
		Port:       int(freeTCPPort(t)),
		TimeoutMs:  10,
		IsTcpFlag:  1,
	})
	if resp.ErrorCode != base.ErrorCodeSucc {
		t.Fatalf("start failed, code=%d desp=%s", resp.ErrorCode, resp.Desp)
	}
	defer manager.Stop(resp.Data.StreamName, "")

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("session was not removed after timeout")
		case <-ticker.C:
			manager.mu.Lock()
			_, ok := manager.sessionsByID[resp.Data.SessionId]
			manager.mu.Unlock()
			if !ok {
				return
			}
		}
	}
}

func TestNewManagerUsesConfiguredPortRangeAfterListenPort(t *testing.T) {
	manager := NewManager(nil, config.GB28181MediaConfig{
		ListenPort:            31000,
		MultiPortMaxIncrement: 10,
	})

	if manager.portMin != 31001 || manager.portMax != 31010 {
		t.Fatalf("port range = [%d,%d], want [31001,31010]", manager.portMin, manager.portMax)
	}
}
