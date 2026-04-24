# API Gateway

`lalmax` 作为 `lal` 的统一 API 网关，对外建议只暴露一个 HTTP 入口。

Hook 体系的详细设计见 [hook_plugin_architecture.md](./hook_plugin_architecture.md)。

默认入口来自：

```json
{
  "lalmax": {
    "http_config": {
      "http_listen_addr": ":1290"
    }
  }
}
```

## Exposed Routes

### Stat

- `GET /api/stat/group`
- `GET /api/stat/all_group`
- `GET /api/stat/lal_info`

### Control

- `POST /api/ctrl/start_relay_pull`
- `GET /api/ctrl/stop_relay_pull`
- `POST /api/ctrl/stop_relay_pull`
- `POST /api/ctrl/kick_session`
- `POST /api/ctrl/start_rtp_pub`
- `POST /api/ctrl/stop_rtp_pub`

### Hook

- `GET /api/hook/recent`
- `GET /api/hook/stream`

## Why Use lalmax Gateway

- `lal` 原生流状态仍由 `lal` 负责，避免双事实源
- `lalmax` 在响应中补充扩展协议订阅者统计
- hook 事件统一从 `lalmax` 读取，不必同时维护 HTTP notify 和内部状态
- 控制接口、查询接口、hook 接口共用一套鉴权策略

## Group Visibility

`lalmax` 获取 group 视图的方式是：

1. 通过内嵌 `lal` 的 `StatAllGroup()` 获取原生 group 快照
2. 通过 `lalmax/logic` 获取扩展订阅者状态
3. 聚合成统一视图后再对外返回或分发到 hook hub

这样 `lalmax` 可以知道 `lal group` 中所有流的原生状态，同时保留自己的扩展消费层状态。

## Stat Response Extension

`/api/stat/group` 和 `/api/stat/all_group` 在保持 `lal` 原有字段的同时，会额外返回一个 `lalmax` 扩展块。

兼容原则如下：

- 原有 `stream_name`、`app_name`、`pub`、`subs`、`pull`、`in_frame_per_sec` 等字段继续保留
- `subs` 仍然表示统一后的订阅者视图，其中会合并 `lal` 原生订阅者和 `lalmax` 扩展订阅者
- `lalmax.ext_subs` 只列出来自 `lalmax` 扩展层的订阅者，便于业务侧区分来源

示例：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {
    "stream_name": "camera01",
    "app_name": "live",
    "pub": {},
    "subs": [
      {
        "session_id": "RTMPSUB1",
        "protocol": "RTMP"
      },
      {
        "session_id": "whep-123",
        "protocol": "WHEP"
      }
    ],
    "pull": {},
    "in_frame_per_sec": [],
    "lalmax": {
      "ext_subs": [
        {
          "session_id": "whep-123",
          "protocol": "WHEP"
        }
      ]
    }
  }
}
```

如果业务只想拿 `lal` 原生兼容视图，可以继续只读原字段；如果业务需要知道 `lalmax` 在该流上维护了哪些扩展订阅者，则读取 `lalmax.ext_subs`。

## Control API Scope

`/api/ctrl/*` 仍然保持轻量控制接口定位，不会在响应中额外塞入完整流状态、订阅者列表或 `lalmax` 扩展统计。

原因是：

- 控制接口的职责是执行动作并返回动作结果
- 流状态属于查询语义，应统一从 `/api/stat/*` 获取
- 避免控制响应膨胀，降低兼容性和调用方解析成本
