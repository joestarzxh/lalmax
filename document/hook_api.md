# Hook API

`lalmax` 统一托管 `lal` 的 notify 事件，并补充 `lalmax` 自身扩展订阅状态。

如果需要理解完整分层、调用链、插件职责和设计边界，见 [hook_plugin_architecture.md](./hook_plugin_architecture.md)。

默认建议：

- 对外状态与控制走 `lalmax` 的 `/api/stat/*` 和 `/api/ctrl/*`
- 对外 hook 事件读取也走 `lalmax`
- `lal.http_api` 和外部业务直接对接 `lal` 原生 notify 只作为调试手段

## Event Source

`lalmax` 内部将以下事件统一写入 hook hub：

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

其中 `on_update` 的 `groups` 数据已经过 `lalmax` 聚合，包含：

- `lal` 原生 group 状态
- `lalmax` 扩展订阅者统计

与 `/api/stat/group`、`/api/stat/all_group` 一样，`on_update.groups[*]` 中的 `subs` 也是统一聚合后的订阅列表；如果业务需要显式区分 `lalmax` 扩展订阅者，建议结合 stat API 中的 `lalmax.ext_subs` 使用。

`on_group_start`、`on_stream_active` 和 `on_group_stop` 是 `lalmax` 基于统一输入流生命周期直接生成的事件，payload 结构如下：

```json
{
  "server_id": "1",
  "app_name": "live",
  "stream_name": "test110"
}
```

注意：当前上游 `lal` 的 `WithOnHookSession` 只直接提供 `streamName`，因此这类 group 生命周期事件里的 `app_name` 在部分场景下可能为空，不能把它当成始终可靠存在的字段。

三者的语义区别是：

- `on_group_start`: 流生命周期进入 `lalmax`
- `on_stream_active`: 收到首个音频或视频 RTMP 消息，只触发一次
- `on_group_stop`: 流生命周期结束。业务上要判断“没有流了”，应使用这个事件

其中“没有流了”不单独新增新的 hook，仍统一使用 `on_group_stop`。

## HTTP API

### `GET /api/hook/recent`

读取最近 hook 事件快照。

请求参数：

- `limit`: 可选，返回事件数量，默认 `20`
- `app_name`: 可选，只返回指定 app 的事件
- `stream_name`: 可选，只返回指定流的事件
- `session_id`: 可选，只返回指定会话的事件
- `event`: 可选，只返回单个事件类型
- `events`: 可选，逗号分隔的多个事件类型

示例：

```bash
curl "http://127.0.0.1:1290/api/hook/recent?limit=5"
curl "http://127.0.0.1:1290/api/hook/recent?stream_name=test110&events=on_group_start,on_stream_active,on_group_stop,on_update"
```

响应示例：

```json
{
  "error_code": 0,
  "desp": "succ",
  "data": {
    "events": [
      {
        "id": 12,
        "event": "on_pub_start",
        "timestamp": "2026-04-24T15:20:11.123456789+08:00",
        "payload": {
          "server_id": "1",
          "session_id": "RTMPPUB1",
          "protocol": "RTMP",
          "base_type": "PUB",
          "stream_name": "test110"
        }
      }
    ]
  }
}
```

### `GET /api/hook/stream`

以 `Server-Sent Events` 持续订阅 hook 事件。

示例：

```bash
curl -N http://127.0.0.1:1290/api/hook/stream
curl -N "http://127.0.0.1:1290/api/hook/stream?stream_name=test110&events=on_group_start,on_stream_active,on_group_stop,on_update"
```

返回格式：

```text
id: 12
event: on_pub_start
data: {"server_id":"1","session_id":"RTMPPUB1","protocol":"RTMP","base_type":"PUB","stream_name":"test110"}
```

连接建立后会先回放最近一批事件，再进入实时流。

## In-Process Usage

如果业务代码和 `lalmax` 在同一进程内，可以直接使用：

```go
hub := serverInstance.HookHub()
_, ch, cancel := hub.Subscribe(64)
defer cancel()

for event := range ch {
    // event.Event
    // event.Payload
}
```

## Plugin Usage

如果具体业务希望由插件处理，而不是把逻辑写进 `lalmax` 主流程，可以注册 hook 插件：

```go
type BizPlugin struct{}

func (p *BizPlugin) Name() string { return "biz-plugin" }

func (p *BizPlugin) OnHookEvent(event server.HookEvent) error {
    // 业务处理
    return nil
}

cancel, err := serverInstance.RegisterHookPlugin(&BizPlugin{}, server.HookPluginOptions{
    Filter: server.NewHookEventFilter("live", "test110", "", []string{
        server.HookEventPubStart,
        server.HookEventPubStop,
    }),
})
if err != nil {
    panic(err)
}
defer cancel()
```

当前默认的 HTTP notify 转发已经作为内置插件存在，外部业务插件只需要关注自己的处理逻辑。

## Notes

- `/api/hook/*` 使用和 `/api/stat/*`、`/api/ctrl/*` 相同的鉴权中间件
- 当前 `lal` 的 `WithOnHookSession` 回调只提供 `streamName`，不提供 `appName`
- 因此扩展订阅者与 `app_name` 的精确归属能力仍受上游 hook 入参限制
- 建议只使用 `lalmax.http_notify` 作为对外 webhook 配置；如果 `lal` 配置段也单独开启原生 `http_notify`，尤其是 `update_interval_sec`，可能出现重复的 `on_update`
- `on_group_start` / `on_stream_active` / `on_group_stop` 比基于 `on_update` 快照 diff 的方案更实时，也更不容易漏掉短生命周期流
- `on_update` 仍然建议保留给状态快照、巡检和最终一致对账使用
