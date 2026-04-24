# lal Native HTTP API

`lalmax` 内嵌运行 `lal`。默认情况下，对外建议统一使用 `lalmax` 自己的 API 门面：

- `lalmax.http_config.http_listen_addr` 下的 `/api/stat/*`
- `lalmax.http_config.http_listen_addr` 下的 `/api/ctrl/*`
- `lalmax.http_config.http_listen_addr` 下的 `/api/hook/*`

`lal.http_api` 只建议在调试 `lal` 原生行为时临时开启。

默认配置：

```json
{
  "http_api": {
    "enable": false,
    "addr": ":8083"
  }
}
```

启用后可访问：

```text
http://127.0.0.1:8083
```

## Native Endpoints

`lal` 原生 HTTP API 当前主要包含：

- `GET /lal.html`
- `GET /api/stat/lal_info`
- `GET /api/stat/all_group`
- `GET /api/stat/group`
- `POST /api/ctrl/start_relay_pull`
- `GET /api/ctrl/stop_relay_pull`
- `POST /api/ctrl/kick_session`
- `POST /api/ctrl/start_rtp_pub`

## Recommended Gateway

推荐直接使用 `lalmax` API，因为它会在 `lal` 原生结果基础上补充：

- `lalmax` 扩展订阅者统计
- 更完整的统一状态视图
- 统一的 hook 事件读取能力
- 统一鉴权入口

默认地址：

```text
http://127.0.0.1:1290/api/stat/group
http://127.0.0.1:1290/api/stat/all_group
http://127.0.0.1:1290/api/stat/lal_info
http://127.0.0.1:1290/api/ctrl/start_relay_pull
http://127.0.0.1:1290/api/ctrl/stop_relay_pull
http://127.0.0.1:1290/api/ctrl/kick_session
http://127.0.0.1:1290/api/ctrl/start_rtp_pub
http://127.0.0.1:1290/api/ctrl/stop_rtp_pub
http://127.0.0.1:1290/api/hook/recent
http://127.0.0.1:1290/api/hook/stream
```

## Compatibility Notes

- `lalmax` 的 `/api/ctrl/*` 请求/响应结构与 `lal` 原生 API 基本保持一致
- `lalmax` 的 `/api/stat/group` 和 `/api/stat/all_group` 会在兼容 `lal` 原有字段的基础上新增 `lalmax` 扩展块
- `stop_relay_pull` 在 `lalmax` 中兼容 `GET`
- `stat/group` 在 `lalmax` 中会优先结合 `app_name + stream_name` 做更精确的 group 匹配
- `on_update` 等 hook 事件在 `lalmax` 中已经过聚合增强

## Stat API Extension

`lalmax` 的统计接口会返回两层信息：

1. `lal` 兼容层
   也就是原有的 `stream_name`、`app_name`、`pub`、`subs`、`pull`、`in_frame_per_sec` 等字段
2. `lalmax` 扩展层
   当前主要是 `lalmax.ext_subs`

其中：

- `subs` 是聚合后的统一订阅列表
- `lalmax.ext_subs` 是其中来自 `lalmax` 扩展协议层的子集

这意味着调用方如果完全按照 `lal` 老接口解析，通常仍然可以工作；如果要区分哪些订阅者是 `lalmax` 自己维护的，就再读取 `lalmax.ext_subs`。

控制类接口不附带这些扩展状态。如果执行控制动作后还需要查看最新流状态，应再调用 `/api/stat/group` 或 `/api/stat/all_group`。

## Debug Usage

只有在以下场景，才建议单独开启 `lal.http_api`：

- 排查 `lal` 原生 HTTP API 行为
- 对比 `lal` 原始 group 数据和 `lalmax` 聚合数据
- 调试上游 `lal` 升级后的兼容性
