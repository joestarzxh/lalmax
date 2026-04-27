# HTTP API 总览

`lalmax` 对外建议统一只暴露一个 HTTP 管理入口，也就是 `lalmax.http_config.http_listen_addr`，示例配置通常是 `:1290`。  
`lal.http_api` 仍然可以保留给排查 `lal` 原生行为时使用，但默认建议关闭，不要和 `lalmax` 的管理入口混用。

建议配合以下文档一起看：

- [api_gateway.md](./api_gateway.md)：统一入口和状态聚合说明
- [hook_api.md](./hook_api.md)：Hook 查询与订阅接口
- [hook_plugin_architecture.md](./hook_plugin_architecture.md)：HookHub 与插件化架构
- [lal_api.md](./lal_api.md)：`lal` 原生 HTTP API 的定位和兼容关系

## 基本约定

- 对外统一入口默认是 `http://127.0.0.1:1290`
- `/api/stat/*`、`/api/ctrl/*`、`/api/hook/*` 共用同一套鉴权配置：`lalmax.http_config.ctrl_auth_whitelist`
- 如果鉴权失败，HTTP 状态码仍然是 `200`，返回体里的 `error_code` 为 `401`
- 除 `GET /api/hook/stream` 之外，其余接口都返回 JSON

统一返回结构如下：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {}
}
```

常见 `error_code` 如下：

| error_code | desp | 说明 |
| --- | --- | --- |
| 0 | succ | 调用成功 |
| 401 | Unauthorized | 鉴权失败 |
| 1001 | group not found | 流分组不存在，或者仅传 `stream_name` 时无法唯一定位 |
| 1002 | param missing | 必填参数缺失 |
| 1003 | session not found | 会话不存在 |
| 2001 | 具体错误信息见 `desp` | `start_relay_pull` 执行失败 |
| 2002 | 具体错误信息见 `desp` | `start_rtp_pub` 执行失败 |

## 当前接口列表

### 统计接口

- `GET /api/stat/group`
- `GET /api/stat/all_group`
- `GET /api/stat/lal_info`

### 控制接口

- `POST /api/ctrl/start_relay_pull`
- `GET /api/ctrl/stop_relay_pull`
- `POST /api/ctrl/stop_relay_pull`
- `POST /api/ctrl/kick_session`
- `POST /api/ctrl/start_rtp_pub`
- `POST /api/ctrl/stop_rtp_pub`

### Hook 接口

- `GET /api/hook/recent`
- `GET /api/hook/stream`

## 统计接口

### `GET /api/stat/group`

查询单个流分组的当前状态。

请求参数：

- `stream_name`：必填，流名
- `app_name`：可选，应用名

说明：

- 如果同一个 `stream_name` 只对应一个流分组，可以只传 `stream_name`
- 如果同一个 `stream_name` 在多个 `app_name` 下都存在，建议同时传 `app_name + stream_name`
- 当前返回结果会在兼容 `lal` 原生字段的基础上，额外补充 `lalmax.ext_subs`

请求示例：

```bash
curl "http://127.0.0.1:1290/api/stat/group?stream_name=test110"
curl "http://127.0.0.1:1290/api/stat/group?app_name=live&stream_name=test110"
```

返回示例：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {
    "stream_name": "test110",
    "app_name": "live",
    "audio_codec": "AAC",
    "video_codec": "H264",
    "video_width": 1920,
    "video_height": 1080,
    "pub": {
      "session_id": "RTMPPUB1",
      "protocol": "RTMP",
      "base_type": "PUB"
    },
    "subs": [
      {
        "session_id": "RTMPSUB1",
        "protocol": "RTMP",
        "base_type": "SUB"
      },
      {
        "session_id": "whep-123",
        "protocol": "WHEP",
        "base_type": "SUB"
      }
    ],
    "pull": {
      "base_type": "PULL"
    },
    "in_frame_per_sec": [],
    "lalmax": {
      "ext_subs": [
        {
          "session_id": "whep-123",
          "protocol": "WHEP",
          "base_type": "SUB"
        }
      ]
    }
  }
}
```

字段语义：

- `subs`：统一后的订阅者视图，包含 `lal` 原生订阅者和 `lalmax` 扩展订阅者
- `lalmax.ext_subs`：只包含 `lalmax` 扩展层维护的订阅者，便于业务侧区分来源

### `GET /api/stat/all_group`

查询当前所有流分组。

请求示例：

```bash
curl "http://127.0.0.1:1290/api/stat/all_group"
```

返回示例：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {
    "groups": [
      {
        "stream_name": "test110",
        "app_name": "live",
        "lalmax": {
          "ext_subs": []
        }
      }
    ]
  }
}
```

其中 `groups[*]` 的结构和 `/api/stat/group` 的 `data` 完全一致。

### `GET /api/stat/lal_info`

查询服务基础信息。

请求示例：

```bash
curl "http://127.0.0.1:1290/api/stat/lal_info"
```

该接口直接返回内嵌 `lal` 的运行信息，常见字段包括：

- `server_id`
- `bin_info`
- `lal_version`
- `api_version`
- `notify_version`
- `start_time`

## 控制接口

### `POST /api/ctrl/start_relay_pull`

让服务主动去远端拉流。

请求体为 JSON，必填字段：

- `url`

常用可选字段：

- `stream_name`
- `pull_timeout_ms`
- `pull_retry_num`
- `auto_stop_pull_after_no_out_ms`
- `rtsp_mode`
- `debug_dump_packet`

当前程序里的默认值如下：

- `pull_timeout_ms` 默认 `10000`
- `pull_retry_num` 默认 `0`
- `auto_stop_pull_after_no_out_ms` 默认 `-1`
- `rtsp_mode` 默认 `0`，也就是 TCP

请求示例：

```bash
curl -H "Content-Type: application/json" \
  -X POST \
  -d "{\"url\":\"rtmp://127.0.0.1/live/test110\"}" \
  "http://127.0.0.1:1290/api/ctrl/start_relay_pull"
```

说明：

- 接口返回成功，只代表命令已经被接受
- 是否真的拉流成功，需要看后续状态接口，或者看 Hook 事件中的 `on_relay_pull_start`、`on_update`

### `GET /api/ctrl/stop_relay_pull`
### `POST /api/ctrl/stop_relay_pull`

关闭指定的 relay pull 会话。

当前实现里，这个接口无论用 `GET` 还是 `POST`，都从查询参数里读取：

- `stream_name`：必填

请求示例：

```bash
curl "http://127.0.0.1:1290/api/ctrl/stop_relay_pull?stream_name=test110"
curl -X POST "http://127.0.0.1:1290/api/ctrl/stop_relay_pull?stream_name=test110"
```

说明：

- 也可以用 `kick_session` 关闭 pull 会话

### `POST /api/ctrl/kick_session`

强制关闭指定会话。

请求体为 JSON，必填字段：

- `stream_name`
- `session_id`

请求示例：

```bash
curl -H "Content-Type: application/json" \
  -X POST \
  -d "{\"stream_name\":\"test110\",\"session_id\":\"FLVSUB1\"}" \
  "http://127.0.0.1:1290/api/ctrl/kick_session"
```

适用对象：

- 推流会话
- 拉流会话
- relay pull 会话

### `POST /api/ctrl/start_rtp_pub`

打开一个 GB28181/RTP 接收会话。

请求体为 JSON，必填字段：

- `stream_name`

常用可选字段：

- `port`
- `timeout_ms`
- `is_tcp_flag`
- `debug_dump_packet`

当前程序里的默认值：

- `timeout_ms` 默认 `60000`

请求示例：

```bash
curl -H "Content-Type: application/json" \
  -X POST \
  -d "{\"stream_name\":\"gb28181-test\",\"port\":0}" \
  "http://127.0.0.1:1290/api/ctrl/start_rtp_pub"
```

说明：

- `port=0` 表示由服务自动分配端口
- 成功后会返回 `session_id` 和最终监听端口

### `POST /api/ctrl/stop_rtp_pub`

关闭 GB28181/RTP 接收会话。

当前实现支持两种传参方式：

- 查询参数：`stream_name` 或 `session_id`
- JSON 请求体：`stream_name` 或 `session_id`

两者至少传一个。

请求示例：

```bash
curl -X POST "http://127.0.0.1:1290/api/ctrl/stop_rtp_pub?stream_name=gb28181-test"
curl -H "Content-Type: application/json" \
  -X POST \
  -d "{\"session_id\":\"PSSUB1\"}" \
  "http://127.0.0.1:1290/api/ctrl/stop_rtp_pub"
```

成功后会返回被关闭的 `session_id`。

## Hook 接口

### `GET /api/hook/recent`

读取最近的 Hook 事件快照。

常用查询参数：

- `limit`：返回条数，默认 `20`
- `app_name`
- `stream_name`
- `session_id`
- `event`
- `events`：多个事件名，逗号分隔

请求示例：

```bash
curl "http://127.0.0.1:1290/api/hook/recent?limit=5"
curl "http://127.0.0.1:1290/api/hook/recent?stream_name=test110&events=on_group_start,on_stream_active,on_group_stop,on_update"
```

### `GET /api/hook/stream`

使用 SSE 持续订阅 Hook 事件。

它和 `/api/hook/recent` 使用同一套过滤参数。连接建立后，会先回放最近一批命中的事件，然后继续推送实时事件。

请求示例：

```bash
curl -N "http://127.0.0.1:1290/api/hook/stream"
curl -N "http://127.0.0.1:1290/api/hook/stream?stream_name=test110&events=on_update,on_group_stop"
```

返回格式示例：

```text
id: 12
event: on_pub_start
data: {"server_id":"1","session_id":"RTMPPUB1","protocol":"RTMP","base_type":"PUB","stream_name":"test110"}
```

当前 Hook 体系里的事件名称、语义、过滤规则和插件化接入方式，请直接参考 [hook_api.md](./hook_api.md) 和 [hook_plugin_architecture.md](./hook_plugin_architecture.md)。
