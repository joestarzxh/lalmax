package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	maxlogic "github.com/q191201771/lalmax/logic"

	config "github.com/q191201771/lalmax/config"

	"github.com/q191201771/lal/pkg/base"
)

var max *LalMaxServer

const httpNotifyAddr = ":55559"

func TestMain(m *testing.M) {
	var err error
	max, err = NewLalMaxServer(&config.Config{
		Fmp4Config: config.Fmp4Config{
			Http: config.Fmp4HttpConfig{Enable: true},
		},
		LalRawContent: []byte(`{"rtmp":{"enable":false},"rtsp":{"enable":false},"http_api":{"enable":false},"pprof":{"enable":false}}`),
		HttpConfig: config.HttpConfig{
			ListenAddr: ":52349",
		},
		HttpNotifyConfig: config.HttpNotifyConfig{
			Enable:            true,
			UpdateIntervalSec: 2,
			OnUpdate:          fmt.Sprintf("http://127.0.0.1%s/on_update", httpNotifyAddr),
		},
	})
	if err != nil {
		panic(err)
	}
	go max.Run()
	os.Exit(m.Run())
}

func TestAllGroup(t *testing.T) {
	_, err := max.lalsvr.AddCustomizePubSession("test")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("no consumer", func(t *testing.T) {
		r := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/stat/all_group", nil)
		max.router.ServeHTTP(r, req)
		resp := r.Result()
		if resp.StatusCode != 200 {
			t.Fatal(resp.Status)
		}
		var out base.ApiStatAllGroupResp
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if len(out.Data.Groups) <= 0 {
			t.Fatal("no group")
		}
		if len(out.Data.Groups[0].StatSubs) != 0 {
			t.Fatal("subs err")
		}
	})

	t.Run("has consumer", func(t *testing.T) {
		ss, _ := maxlogic.GetGroupManagerInstance().GetOrCreateGroupByStreamName("test", "test", max.hlssvr, 1, 0)
		ss.AddConsumer("consumer1", nil)

		r := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/stat/all_group", nil)
		max.router.ServeHTTP(r, req)
		resp := r.Result()
		if resp.StatusCode != 200 {
			t.Fatal(resp.Status)
		}
		var out base.ApiStatAllGroupResp
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if len(out.Data.Groups) <= 0 {
			t.Fatal("no group")
		}
		if len(out.Data.Groups[0].StatSubs) <= 0 {
			t.Fatal("subs err")
		}
		group := out.Data.Groups[0]
		if group.StatSubs[0].SessionId != "consumer1" {
			t.Fatal("SessionId err")
		}
	})
}

func TestNotifyUpdate(t *testing.T) {
	streamName := "notify_test"
	consumerID := "consumer_notify"

	_, err := max.lalsvr.AddCustomizePubSession(streamName)
	if err != nil {
		t.Fatal(err)
	}
	ss, _ := maxlogic.GetGroupManagerInstance().GetOrCreateGroupByStreamName(streamName, streamName, max.hlssvr, 1, 0)
	ss.AddConsumer(consumerID, nil)

	http.HandleFunc("/on_update", func(w http.ResponseWriter, r *http.Request) {
		var out base.ApiStatAllGroupResp
		if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		for _, group := range out.Data.Groups {
			for _, sub := range group.StatSubs {
				if sub.SessionId == consumerID {
					return
				}
			}
		}
		t.Fatal("SessionId err")
	})
	go http.ListenAndServe(httpNotifyAddr, nil)
	time.Sleep(time.Second * 3)
}

func TestRtpPubStartStop(t *testing.T) {
	body := bytes.NewBufferString(`{"stream_name":"rtp_pub_test","port":0,"timeout_ms":0}`)
	r := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ctrl/start_rtp_pub", body)
	max.router.ServeHTTP(r, req)
	resp := r.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.Status)
	}

	var startResp base.ApiCtrlStartRtpPubResp
	if err := json.NewDecoder(resp.Body).Decode(&startResp); err != nil {
		t.Fatal(err)
	}
	if startResp.ErrorCode != base.ErrorCodeSucc {
		t.Fatalf("start_rtp_pub failed, code=%d desp=%s", startResp.ErrorCode, startResp.Desp)
	}
	if startResp.Data.StreamName != "rtp_pub_test" || startResp.Data.SessionId == "" || startResp.Data.Port == 0 {
		t.Fatalf("unexpected start_rtp_pub data: %+v", startResp.Data)
	}

	r = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/ctrl/stop_rtp_pub?stream_name=rtp_pub_test", nil)
	max.router.ServeHTTP(r, req)
	resp = r.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.Status)
	}

	var stopResp base.ApiCtrlStopRelayPullResp
	if err := json.NewDecoder(resp.Body).Decode(&stopResp); err != nil {
		t.Fatal(err)
	}
	if stopResp.ErrorCode != base.ErrorCodeSucc {
		t.Fatalf("stop_rtp_pub failed, code=%d desp=%s", stopResp.ErrorCode, stopResp.Desp)
	}
	if stopResp.Data.SessionId != startResp.Data.SessionId {
		t.Fatalf("stop_rtp_pub session id = %s, want %s", stopResp.Data.SessionId, startResp.Data.SessionId)
	}
}

func TestAuthentication(t *testing.T) {
	t.Run("无须鉴权", func(t *testing.T) {
		if !authentication("12", "192.168.0.2", nil, nil) {
			t.Fatal("期望通过， 但实际未通过")
		}
	})
	t.Run("Token 鉴权失败", func(t *testing.T) {
		if authentication("1", "192.168.0.2", []string{"12"}, nil) {
			t.Fatal("期望不通过， 但实际通过")
		}
	})
	t.Run("token 鉴权成功", func(t *testing.T) {
		if !authentication("12", "192.168.0.2", []string{"12"}, nil) {
			t.Fatal("期望通过， 但实际不通过")
		}
	})
	t.Run("ip 白名单鉴权失败", func(t *testing.T) {
		if authentication("12", "192.168.0.2", nil, []string{"192.168.1.2"}) {
			t.Fatal("期望不通过， 但实际通过")
		}
	})
	t.Run("ip 白名单鉴权成功", func(t *testing.T) {
		if !authentication("12", "192.168.0.2", []string{"12"}, []string{"192.168.0.2"}) {
			t.Fatal("期望通过， 但实际不通过")
		}
	})
	t.Run("两种模式结合鉴权通过", func(t *testing.T) {
		if !authentication("12", "192.168.0.2", []string{"12"}, []string{"192.168.0.2"}) {
			t.Fatal("期望通过， 但实际不通过")
		}
	})
}
