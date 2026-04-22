package config

import (
	"strings"
	"testing"
)

func TestUnmarshalStructuredConfig(t *testing.T) {
	raw := []byte(`{
		"lalmax": {
			"srt_config": {
				"enable": true,
				"addr": ":6001"
			},
			"server_id": "lalmax-1"
		},
		"lal": {
			"rtmp": {
				"enable": true,
				"addr": ":1935"
			}
		}
	}`)

	if err := Unmarshal(raw); err != nil {
		t.Fatalf("unmarshal structured config: %v", err)
	}

	cfg := GetConfig()
	if !cfg.SrtConfig.Enable || cfg.SrtConfig.Addr != ":6001" {
		t.Fatalf("unexpected srt config: %+v", cfg.SrtConfig)
	}
	if cfg.ServerId != "lalmax-1" {
		t.Fatalf("unexpected server id: %s", cfg.ServerId)
	}
	if !strings.Contains(string(cfg.LalRawContent), `"rtmp"`) {
		t.Fatalf("lal raw content not preserved: %s", string(cfg.LalRawContent))
	}
}

func TestUnmarshalLegacyConfig(t *testing.T) {
	raw := []byte(`{
		"srt_config": {
			"enable": true,
			"addr": ":6001"
		},
		"lal_config_path:": "./conf/lalserver.conf.json"
	}`)

	if err := Unmarshal(raw); err != nil {
		t.Fatalf("unmarshal legacy config: %v", err)
	}

	cfg := GetConfig()
	if !cfg.SrtConfig.Enable || cfg.SrtConfig.Addr != ":6001" {
		t.Fatalf("unexpected srt config: %+v", cfg.SrtConfig)
	}
	if cfg.LalSvrConfigPath != "./conf/lalserver.conf.json" {
		t.Fatalf("unexpected lal config path: %s", cfg.LalSvrConfigPath)
	}
	if len(cfg.LalRawContent) != 0 {
		t.Fatalf("legacy config should not set lal raw content: %s", string(cfg.LalRawContent))
	}
}
