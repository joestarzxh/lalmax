# lalmax 配置说明

本文档说明 `conf/lalmax.conf.json` 里 `lalmax` 这一段的配置。  
`lal` 原生配置请看 [lal_config.md](./lal_config.md)。

## 推荐配置结构

当前程序推荐使用一个统一的配置文件，并按顶层标签拆开：

```json
{
  "lalmax": {
    "server_id": "1",
    "srt_config": {},
    "rtc_config": {},
    "http_config": {},
    "fmp4_config": {},
    "logic_config": {},
    "http_notify": {},
    "gb28181_config": {}
  },
  "lal": {}
}
```

说明：

- `lalmax`：`lalmax` 自己的扩展能力配置
- `lal`：内嵌 `lal` 的原生配置

如果同时提供了顶层 `lal` 标签，程序会优先使用这段内容作为 `lal` 的原生配置，不再读取 `lal_config_path` 指向的文件。

## srt_config

SRT 服务配置。

- `enable`：是否启用 SRT
- `addr`：SRT 监听地址，示例 `:6001`

示例：

```json
{
  "enable": true,
  "addr": ":6001"
}
```

## rtc_config

RTC 服务配置。目前主要用于 WHIP、WHEP 和 Jessibuca 播放链路。

- `enable`：是否启用 RTC
- `ice_host_nat_to_ips`：对外暴露的 ICE 地址列表；为空时使用本机可用地址
- `ice_udp_mux_port`：ICE UDP 复用端口
- `ice_tcp_mux_port`：ICE TCP 复用端口
- `write_chan_size`：RTC 订阅侧写队列大小；如果填 `0`，程序会自动使用 `1024`

示例：

```json
{
  "enable": true,
  "ice_host_nat_to_ips": ["192.168.0.1"],
  "ice_udp_mux_port": 4888,
  "ice_tcp_mux_port": 4888,
  "write_chan_size": 1024
}
```

## http_config

`lalmax` 自己的 HTTP/HTTPS 配置。管理接口、RTC 信令、HTTP-FMP4、HLS-FMP4/LLHLS 都依赖这里。

- `http_listen_addr`：HTTP 监听地址，示例 `:1290`
- `enable_https`：是否启用 HTTPS
- `https_listen_addr`：HTTPS 监听地址
- `https_cert_file`：HTTPS 证书文件
- `https_key_file`：HTTPS 私钥文件
- `ctrl_auth_whitelist`：管理接口鉴权配置

`ctrl_auth_whitelist` 的字段如下：

- `secrets`：允许的令牌列表，请求时通过查询参数 `token` 传入
- `ips`：允许访问的客户端 IP 列表

当前鉴权覆盖范围：

- `/api/stat/*`
- `/api/ctrl/*`
- `/api/hook/*`

规则说明：

- 如果 `secrets` 和 `ips` 都为空，表示不做鉴权
- 如果两者都配置了，请求必须同时满足两项
- 鉴权失败时，HTTP 状态码仍然是 `200`，返回体里的 `error_code` 是 `401`

示例：

```json
{
  "http_listen_addr": ":1290",
  "enable_https": true,
  "https_listen_addr": ":1233",
  "https_cert_file": "./conf/cert.pem",
  "https_key_file": "./conf/key.pem",
  "ctrl_auth_whitelist": {
    "ips": ["192.168.1.10"],
    "secrets": ["EC3D1536-5D93-4BD6-9FBD-96A52CB1596D"]
  }
}
```

## fmp4_config

`lalmax` 的 FMP4 相关配置，分成 `http` 和 `hls` 两段。

### fmp4_config.http

HTTP-FMP4 配置。

- `enable`：是否启用 HTTP-FMP4

### fmp4_config.hls

HLS-FMP4 / LLHLS 配置。

- `enable`：是否启用 HLS-FMP4 / LLHLS
- `segment_count`：m3u8 保留的切片数量
- `segment_duration`：切片时长，单位秒
- `part_duration`：LLHLS part 时长，单位毫秒
- `low_latency`：是否启用低延迟 HLS

示例：

```json
{
  "http": {
    "enable": true
  },
  "hls": {
    "enable": true,
    "segment_count": 7,
    "segment_duration": 1,
    "part_duration": 200,
    "low_latency": false
  }
}
```

## logic_config

`lalmax` 扩展流分组配置。

- `gop_cache_num`：GOP 缓存数量
- `single_gop_max_frame_num`：单个 GOP 最多缓存多少帧；`0` 表示自动判断

示例：

```json
{
  "gop_cache_num": 1,
  "single_gop_max_frame_num": 0
}
```

## server_id

服务实例标识。

这个值会出现在：

- Hook 事件的 `server_id`
- HTTP 回调的 payload

示例：

```json
"server_id": "1"
```

## http_notify

内置 HTTP 回调插件配置。

先说明两件事：

- 这段配置控制的是“是否向外发 HTTP 回调”
- 不影响内部 HookHub、本地插件注册、`/api/hook/*` 查询和订阅能力

字段如下：

- `enable`：是否启用内置 HTTP 回调插件
- `update_interval_sec`：周期性生成 `on_update` 事件的间隔秒数
- `on_server_start`
- `on_update`
- `on_group_start`
- `on_group_stop`
- `on_stream_active`
- `on_pub_start`
- `on_pub_stop`
- `on_sub_start`
- `on_sub_stop`
- `on_relay_pull_start`
- `on_relay_pull_stop`
- `on_rtmp_connect`
- `on_hls_make_ts`

关于 `update_interval_sec`，当前程序的行为是：

- 大于 `0` 时，`lalmax` 会按这个周期向 HookHub 发布 `on_update`
- 即使 `enable=false`，这些事件依然会进入 HookHub，也能被 `/api/hook/*` 和进程内插件看到
- 只有在 `enable=true` 且对应回调地址非空时，内置插件才会真正向外发 HTTP 请求

示例：

```json
{
  "enable": true,
  "update_interval_sec": 5,
  "on_update": "http://127.0.0.1:10101/on_update",
  "on_group_start": "http://127.0.0.1:10101/on_group_start",
  "on_group_stop": "http://127.0.0.1:10101/on_group_stop",
  "on_stream_active": "http://127.0.0.1:10101/on_stream_active",
  "on_pub_start": "http://127.0.0.1:10101/on_pub_start",
  "on_pub_stop": "http://127.0.0.1:10101/on_pub_stop",
  "on_sub_start": "http://127.0.0.1:10101/on_sub_start",
  "on_sub_stop": "http://127.0.0.1:10101/on_sub_stop",
  "on_relay_pull_start": "http://127.0.0.1:10101/on_relay_pull_start",
  "on_relay_pull_stop": "http://127.0.0.1:10101/on_relay_pull_stop",
  "on_rtmp_connect": "http://127.0.0.1:10101/on_rtmp_connect",
  "on_server_start": "http://127.0.0.1:10101/on_server_start",
  "on_hls_make_ts": "http://127.0.0.1:10101/on_hls_make_ts"
}
```

建议：

- 对外统一只配置 `lalmax.http_notify`
- `lal` 配置段里的原生 `http_notify` 建议保持关闭
- 如果两边同时往外发，尤其都带 `on_update`，很容易出现重复回调

Hook 事件的具体语义请看 [hook_api.md](./hook_api.md)。

## gb28181_config

GB28181 服务配置。

字段如下：

- `enable`：是否启用 GB28181
- `listen_addr`：SIP 服务监听 IP，默认会补成 `0.0.0.0`
- `sip_ip`：SIP 对外地址，生成设备交互内容时会用到
- `sip_port`：SIP 端口，默认 `5060`
- `serial`：平台 ID，默认 `34020000002000000001`
- `realm`：平台域，默认 `3402000000`
- `username`：认证用户名
- `password`：认证密码
- `keepalive_interval`：设备心跳周期，默认 `60`
- `quick_login`：是否允许设备通过 Keepalive 快速建档
- `media_config`：媒体端口配置

`media_config` 字段如下：

- `media_ip`：在 SDP 中对外声明的媒体 IP；默认 `0.0.0.0`
- `listen_port`：固定媒体端口起点；默认 `30000`
- `multi_port_max_increment`：多端口模式下可分配的附加端口范围；默认 `3000`

示例：

```json
{
  "enable": true,
  "listen_addr": "0.0.0.0",
  "sip_ip": "100.100.100.101",
  "sip_port": 5060,
  "serial": "34020000002000000001",
  "realm": "3402000000",
  "username": "admin",
  "password": "admin123",
  "keepalive_interval": 60,
  "quick_login": false,
  "media_config": {
    "media_ip": "100.100.100.101",
    "listen_port": 30000,
    "multi_port_max_increment": 3000
  }
}
```

## 兼容说明

当前程序还兼容一部分旧配置写法：

- 旧版平铺配置仍然可以读
- `lal_config_path` 仍然保留兼容
- 如果没有 `logic_config`，会尝试兼容旧字段 `hook_config`
- 如果没有 `fmp4_config`，会尝试兼容旧字段 `httpfmp4_config` 和 `hls_config`

但新项目建议统一使用当前这套结构，也就是：

- 顶层使用 `lalmax` 和 `lal`
- `lalmax` 内部使用当前代码里的 snake_case 字段名

## 相关文档

- [lal_config.md](./lal_config.md)：`lal` 原生配置
- [api.md](./api.md)：统一管理 API 总览
- [hook_api.md](./hook_api.md)：Hook 查询与订阅接口
- [hook_plugin_architecture.md](./hook_plugin_architecture.md)：HookHub 与插件化架构
