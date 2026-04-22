# lal 原生 HTTP API

本文档说明 `conf/lalmax.conf.json` 中 `lal.http_api` 暴露的 lal 原生 HTTP API。默认配置为：

```json
{
  "http_api": {
    "enable": true,
    "addr": ":8083"
  }
}
```

默认访问地址：

```text
http://127.0.0.1:8083
```

lalmax 自身也在 `lalmax.http_config.http_listen_addr` 上提供 `/api/stat` 和 `/api/ctrl` 兼容接口，并会补充 lalmax 扩展订阅信息。只需要管理 lal 原生流状态时，可以直接使用本文档中的 lal 原生 API。

## 通用响应

所有 API 都返回 JSON，基础结构如下：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {}
}
```

常见 `error_code`：

| error_code | desp | 说明 |
| --- | --- | --- |
| 0 | succ | 调用成功 |
| 404 | page not found | API 路径不存在 |
| 1001 | group not found | 流分组不存在 |
| 1002 | param missing | 必填参数缺失 |
| 1003 | session not found | 会话不存在 |
| 2001 | 失败原因见 desp | `start_relay_pull` 失败 |
| 2002 | 失败原因见 desp | `start_rtp_pub` 监听端口失败 |

## Web UI

### `GET /lal.html`

返回 lal 原生 Web UI 页面。

```bash
curl http://127.0.0.1:8083/lal.html
```

## 查询接口

### `GET /api/stat/lal_info`

查询 lal 服务信息。

```bash
curl http://127.0.0.1:8083/api/stat/lal_info
```

响应示例：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {
    "server_id": "1",
    "bin_info": "GitTag=unknown. GitCommitLog=unknown.",
    "lal_version": "v0.37.4",
    "api_version": "v0.1.2",
    "notify_version": "v0.0.4",
    "WebUiVersion": "",
    "start_time": "2026-04-22 10:00:00.000"
  }
}
```

### `GET /api/stat/all_group`

查询所有流分组。

```bash
curl http://127.0.0.1:8083/api/stat/all_group
```

响应中的 `data.groups` 是数组，每个元素结构与 `/api/stat/group` 的 `data` 相同。

### `GET /api/stat/group`

查询指定流分组。

```bash
curl "http://127.0.0.1:8083/api/stat/group?stream_name=test110"
```

请求参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `stream_name` | 是 | 流名称 |
| `app_name` | 否 | lalmax 兼容 API 可用于精确匹配扩展订阅者；lal 原生 API 当前仍主要按 `stream_name` 查询 |

响应示例：

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
      "session_id": "RTMPPUBSUB1",
      "protocol": "RTMP",
      "base_type": "PUB",
      "remote_addr": "127.0.0.1:50000",
      "start_time": "2026-04-22 10:00:00",
      "read_bytes_sum": 1024,
      "wrote_bytes_sum": 0,
      "bitrate_kbits": 800,
      "read_bitrate_kbits": 800,
      "write_bitrate_kbits": 0
    },
    "subs": [],
    "pull": {
      "session_id": "",
      "protocol": "",
      "base_type": ""
    },
    "in_frame_per_sec": []
  }
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `stream_name` | 流名称 |
| `app_name` | 应用名或路径前缀 |
| `audio_codec` | 音频编码，例如 `AAC`、`PCMA`、`PCMU`、`OPUS` |
| `video_codec` | 视频编码，例如 `H264`、`H265` |
| `pub` | 推流会话统计 |
| `subs` | 拉流会话统计数组 |
| `pull` | 回源拉流会话统计 |
| `in_frame_per_sec` | 输入帧率采样 |

会话统计字段：

| 字段 | 说明 |
| --- | --- |
| `session_id` | 会话唯一标识 |
| `protocol` | 协议，例如 `RTMP`、`RTSP`、`FLV`、`TS` |
| `base_type` | 会话类型，常见为 `PUB`、`SUB`、`PULL` |
| `remote_addr` | 对端地址 |
| `start_time` | 会话开始时间 |
| `read_bytes_sum` | 累计读取字节数 |
| `wrote_bytes_sum` | 累计写出字节数 |
| `bitrate_kbits` | 统计周期内码率，单位 kbit/s |
| `read_bitrate_kbits` | 统计周期内读取码率，单位 kbit/s |
| `write_bitrate_kbits` | 统计周期内写出码率，单位 kbit/s |

## 控制接口

### `POST /api/ctrl/start_relay_pull`

让 lal 主动从远端拉流到本地。

```bash
curl -H "Content-Type: application/json" \
  -X POST \
  -d '{"url":"rtmp://127.0.0.1/live/test110","pull_retry_num":0}' \
  http://127.0.0.1:8083/api/ctrl/start_relay_pull
```

请求参数：

| 参数 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `url` | 是 | 无 | 远端拉流地址，支持 RTMP、RTSP |
| `stream_name` | 否 | 从 `url` 解析 | 本地流名称 |
| `pull_timeout_ms` | 否 | `10000` | 建立拉流连接的超时时间 |
| `pull_retry_num` | 否 | `0` | 重试次数，`-1` 表示一直重试，`0` 表示不重试 |
| `auto_stop_pull_after_no_out_ms` | 否 | `-1` | 没有输出订阅时自动停止拉流，`-1` 表示关闭 |
| `rtsp_mode` | 否 | `0` | RTSP 拉流模式，`0` 为 TCP，`1` 为 UDP |
| `debug_dump_packet` | 否 | 空字符串 | 调试用抓包文件路径，生产环境建议为空 |

响应示例：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {
    "stream_name": "test110",
    "session_id": "RTMPPULL1"
  }
}
```

注意：返回成功只表示命令已被接受，不保证远端流已经拉取成功。实际状态可通过 `/api/stat/group` 或 HTTP Notify 判断。

### `GET /api/ctrl/stop_relay_pull`

停止指定流的回源拉流。

```bash
curl "http://127.0.0.1:8083/api/ctrl/stop_relay_pull?stream_name=test110"
```

请求参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `stream_name` | 是 | 需要停止回源拉流的流名称 |

响应示例：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {
    "session_id": "RTMPPULL1"
  }
}
```

### `POST /api/ctrl/kick_session`

关闭指定会话。会话可以是推流、拉流或回源拉流。

```bash
curl -H "Content-Type: application/json" \
  -X POST \
  -d '{"stream_name":"test110","session_id":"FLVSUB1"}' \
  http://127.0.0.1:8083/api/ctrl/kick_session
```

请求参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `stream_name` | 是 | 流名称 |
| `session_id` | 是 | 会话唯一标识，可从 `/api/stat/group` 获取 |

响应示例：

```json
{
  "error_code": 0,
  "desp": "succ"
}
```

### `POST /api/ctrl/start_rtp_pub`

打开 RTP/PS 接收端口。常用于 GB28181 或外部系统向 lal 投递 RTP/PS 流。

```bash
curl -H "Content-Type: application/json" \
  -X POST \
  -d '{"stream_name":"test110","port":0,"timeout_ms":60000,"is_tcp_flag":0}' \
  http://127.0.0.1:8083/api/ctrl/start_rtp_pub
```

请求参数：

| 参数 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `stream_name` | 是 | 无 | 绑定到 lal 内部的流名称 |
| `port` | 否 | `0` | 接收端口，`0` 表示自动分配 |
| `timeout_ms` | 否 | `60000` | 超时时间，`0` 表示不超时 |
| `is_tcp_flag` | 否 | `0` | `0` 表示 UDP，`1` 表示 TCP |
| `debug_dump_packet` | 否 | 空字符串 | 调试用抓包文件路径，生产环境建议为空 |

响应示例：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {
    "stream_name": "test110",
    "session_id": "PSSUB1",
    "port": 20000
  }
}
```

## lalmax 兼容 API

lalmax 在自己的 HTTP 服务上也提供兼容接口，默认地址来自 `lalmax.http_config.http_listen_addr`：

```text
http://127.0.0.1:1290/api/stat/group
http://127.0.0.1:1290/api/stat/all_group
http://127.0.0.1:1290/api/stat/lal_info
http://127.0.0.1:1290/api/ctrl/start_relay_pull
http://127.0.0.1:1290/api/ctrl/stop_relay_pull
http://127.0.0.1:1290/api/ctrl/kick_session
http://127.0.0.1:1290/api/ctrl/start_rtp_pub
```

lalmax 兼容 API 的请求和响应结构与 lal 原生 API 基本一致，但会在统计结果中补充 lalmax 扩展订阅者信息。控制类接口还可能受 `lalmax.http_config.ctrl_auth_whitelist` 限制。

## 鉴权说明

lal 原生 HTTP API 本身不使用 `simple_auth` 中的流鉴权配置。`simple_auth` 主要控制 RTMP、RTSP、HTTP-FLV、HTTP-TS、HLS 等流访问鉴权。

如果启用流鉴权，请在流地址参数中携带：

```text
lal_secret=<md5(key + streamName)>
```

如果配置了 `dangerous_lal_secret`，也可以直接传：

```text
lal_secret=<dangerous_lal_secret>
```
