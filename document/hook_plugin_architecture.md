# Hook Plugin Architecture

本文档详细说明 `lalmax` 当前的 Hook 体系设计，包括：

- 为什么需要由 `lalmax` 统一托管 hook
- 事件从 `lal` 到业务插件的完整调用链
- `HookHub`、过滤器、插件调度器各自的职责
- 默认 HTTP notify 在新架构中的位置
- 业务插件的推荐接入方式
- 当前设计边界与后续演进方向

## 1. 设计目标

这套 Hook 架构的目标不是把业务逻辑写进 `lalmax`，而是把 `lalmax` 固定为一个稳定的媒体事件平台层。

核心目标：

- `lal` 继续作为原生流状态事实源
- `lalmax` 统一聚合原生状态和扩展订阅状态
- `lalmax` 统一对外暴露 Hook 读取能力
- 具体业务处理通过插件完成，而不是散落在主流程中
- 慢业务不能阻塞媒体主链路

一句话概括：

`lalmax` 负责“采集、聚合、过滤、分发”，业务插件负责“消费和处理”。

## 2. 分层结构

当前 Hook 链路分为 4 层。

### 2.1 `lal` 原生事件层

`lal` 通过 `INotifyHandler` 向外抛出原生事件，例如：

- `OnServerStart`
- `OnUpdate`
- `OnPubStart`
- `OnPubStop`
- `OnSubStart`
- `OnSubStop`
- `OnRelayPullStart`
- `OnRelayPullStop`
- `OnRtmpConnect`
- `OnHlsMakeTs`

这一层只负责产生事件，不负责业务分发。

在 `lalmax` 这一层，还会基于统一输入流生命周期派生额外的 group 生命周期事件：

- `on_group_start`
- `on_stream_active`
- `on_group_stop`

### 2.2 `lalmax` HookHub 层

`lalmax` 使用 [http_notify.go](./../server/http_notify.go) 中的 `HttpNotify` 作为统一 HookHub。

它当前承担 5 类职责：

1. 接住 `lal` 发出的原生 notify 事件
2. 对 `on_update` 的 group 数据做聚合增强
3. 为事件补充过滤所需的元数据
4. 将事件写入历史缓存，并提供 SSE/Recent 读取
5. 将事件异步分发给插件

虽然这个结构体名字仍叫 `HttpNotify`，但职责已经不只是“发 HTTP 回调”，而是整个 Hook 总线。

### 2.3 过滤层

过滤逻辑在 [hook_filter.go](./../server/hook_filter.go)。

这层负责统一定义事件匹配规则，当前支持：

- `app_name`
- `stream_name`
- `session_id`
- `event`
- `events`

这一层的意义是“统一语义”，保证：

- `/api/hook/recent`
- `/api/hook/stream`
- 业务插件注册过滤

三者使用同一套过滤规则，而不是每处自己实现一套判断逻辑。

### 2.4 插件层

插件接口在 [hook_plugin.go](./../server/hook_plugin.go)：

```go
type HookPlugin interface {
    Name() string
    OnHookEvent(event HookEvent) error
}
```

插件层只关心一件事：收到匹配事件后做自己的业务处理。

典型插件可以是：

- HTTP webhook 转发
- Kafka 生产者
- Redis Stream 写入器
- 数据库落表
- 业务内存回调
- 审计日志插件

## 3. 事件调用链

以 `OnPubStart` 为例，完整调用链如下：

```text
lal native event
  -> HttpNotify.NotifyPubStart(info)
  -> publish(HookEventPubStart, info)
  -> 填充过滤元数据
  -> 写入 history
  -> 推送给 SSE / recent 订阅者
  -> dispatchPlugins(event)
  -> 匹配到的插件各自异步消费
```

以 `OnUpdate` 为例，还会多一步聚合：

```text
lal native update
  -> HttpNotify.NotifyUpdate(info)
  -> 聚合 lal group + lalmax 扩展订阅者
  -> publish(HookEventUpdate, mergedInfo)
  -> history / SSE / plugin dispatch
```

而 `on_group_start` / `on_stream_active` / `on_group_stop` 并不是在 `OnUpdate` 流程内 diff 生成的，而是直接跟随输入流生命周期与首个媒体消息触发：

```text
group/media lifecycle
  -> WithOnHookSession create
  -> Group.OnMsg first real media
  -> Group.OnStop
  -> publish(HookEventGroupStart / HookEventStreamActive / HookEventGroupStop, info)
```

这意味着：

- 查询接口拿到的是聚合后的视图
- hook 事件里的 `on_update` 也是聚合后的视图
- HTTP notify 与插件消费看到的是同一份增强数据

## 4. 为什么默认 HTTP notify 也做成插件

旧模式下，`NotifyPubStart/NotifyUpdate/...` 会直接在主流程里发 HTTP POST。

这样做的问题是：

- HTTP 转发是业务出口的一种，不应该写死在主流程
- 后续增加 Kafka、Redis、数据库 sink 时会继续污染主流程
- 不同业务出口的生命周期与重试策略难以统一管理

现在的做法是：

- 主流程只负责 `publish`
- 默认 HTTP notify 转发实现为内置插件
- 内置插件文件在 [hook_builtin_http_plugin.go](./../server/hook_builtin_http_plugin.go)

这样后续无论新增什么业务出口，都和默认 HTTP notify 处于同一层级。

## 5. HookEvent 结构说明

对外公开的事件结构是：

```go
type HookEvent struct {
    ID        int64
    Event     string
    Timestamp string
    Payload   json.RawMessage
}
```

其中：

- `ID` 用于事件顺序控制
- `Event` 是事件类型名，例如 `on_pub_start`
- `Timestamp` 是事件产生时间
- `Payload` 是具体事件数据

此外，内部还会维护用于过滤的元数据，例如：

- `sessionID`
- `streamName`
- `appName`
- `groupKeys`

这些字段不直接暴露给外部 API，但会用于：

- 路由层过滤
- 插件过滤
- `on_update` 的 group 命中判断

## 6. 过滤语义

过滤规则统一由 `HookEventFilter.Match` 决定。

### 6.1 单会话事件

例如：

- `on_group_start`
- `on_stream_active`
- `on_group_stop`
- `on_pub_start`
- `on_pub_stop`
- `on_sub_start`
- `on_sub_stop`
- `on_relay_pull_start`
- `on_relay_pull_stop`

除 group 级事件外，这类事件会直接携带：

- `session_id`
- `stream_name`
- `app_name`

因此过滤时按单个流或单个会话精准匹配。

其中 `on_group_start` / `on_stream_active` / `on_group_stop` 是 group 级别事件，没有 `session_id`，只携带：

- `stream_name`
- `app_name`

其中 `app_name` 当前并不保证始终非空，它仍受上游 `WithOnHookSession` 只提供 `streamName` 的限制。

### 6.2 `on_update`

`on_update` 一次可能携带多个 group。

因此内部会把它展开成一组 `groupKeys`，过滤时判断：

- 是否有任意一个 group 命中过滤条件

也就是说，一个 `on_update` 事件只要包含目标流，就会被保留。

`on_group_start` / `on_stream_active` / `on_group_stop` 是直接跟随输入流生命周期产生的，因此比基于 `on_update` 快照 diff 的方案更实时，也更不容易漏掉短生命周期流。

其中：

- `on_group_start` 表示 group 生命周期开始
- `on_stream_active` 表示首个音频或视频消息真正到达，只触发一次
- `on_group_stop` 表示 group 生命周期结束，也是“没有流了”应使用的事件

但当前仍有一个边界：

- 上游 `lal` 的 `WithOnHookSession` 只提供 `streamName`
- 因此这类 direct lifecycle hook 的 `app_name` 归属能力仍受上游接口限制
- 如果同时保留 `lal` 原生 `http_notify` 和 `lalmax` 自己的 HookHub 出口，尤其同时配置两个 `update_interval_sec`，`on_update` 可能重复

### 6.3 当前支持的过滤条件

- `app_name`
- `stream_name`
- `session_id`
- `event`
- `events`

建议：

- 单流订阅优先同时带 `app_name + stream_name`
- 精确追踪某个连接时使用 `session_id`
- 降低噪音时优先限制 `event/events`

## 7. 业务插件如何接入

业务代码和 `lalmax` 同进程时，推荐直接注册插件。

### 7.1 最小插件示例

```go
type BizPlugin struct{}

func (p *BizPlugin) Name() string {
    return "biz-plugin"
}

func (p *BizPlugin) OnHookEvent(event server.HookEvent) error {
    // 业务处理
    return nil
}
```

### 7.2 注册示例

```go
cancel, err := serverInstance.RegisterHookPlugin(&BizPlugin{}, server.HookPluginOptions{
    Filter: server.NewHookEventFilter("live", "test110", "", []string{
        server.HookEventPubStart,
        server.HookEventPubStop,
        server.HookEventUpdate,
    }),
    BufferSize: 64,
})
if err != nil {
    panic(err)
}
defer cancel()
```

### 7.3 字段说明

- `Name()`
  用作插件唯一标识。重复名称不允许重复注册。

- `Filter`
  用于控制这个插件只消费自己关心的事件。

- `BufferSize`
  用于控制插件异步队列大小。

### 7.4 为什么推荐插件而不是直接改主流程

因为主流程的职责应该稳定，而业务处理天然是变化的。

如果把每个业务都写进主流程，会出现：

- 发布一个新业务就要改核心代码
- 多业务逻辑互相影响
- 回归成本越来越高
- 业务异常更容易污染核心链路

插件化之后，核心层和业务层边界清晰很多。

## 8. 插件调度模型

插件分发是异步的，每个插件有自己的缓冲队列。

调度模型：

```text
publish(event)
  -> 遍历已注册插件
  -> 根据 Filter 判断是否命中
  -> 命中则投递到该插件自己的 queue
  -> 插件 goroutine 从 queue 中消费
```

这个模型的含义是：

- 插件之间互不阻塞
- 插件不会反压媒体主链路
- 某个慢插件只影响自己

当前策略下，如果插件队列满了：

- 当前事件会被丢弃
- 记录 warn 日志

这是有意选择，优先保证媒体主链路稳定。

## 9. 当前默认行为

当前系统启动后，默认会注册一个内置插件：

- `builtin-http-notify`

它负责把事件按旧配置转发到：

- `on_server_start`
- `on_update`
- `on_group_start`
- `on_stream_active`
- `on_group_stop`
- `on_pub_start`
- `on_pub_stop`
- `on_sub_start`
- `on_sub_stop`
- `on_relay_pull_start`
- `on_relay_pull_stop`
- `on_rtmp_connect`
- `on_hls_make_ts`

这意味着旧的 `http_notify` 配置仍然可用，但实现方式已经改成：

```text
HookHub -> builtin-http-notify plugin -> HTTP callback
```

而不再是主流程直接发 HTTP。

## 10. API、SSE、插件三者关系

三者读的是同一个 HookHub。

### 10.1 `/api/hook/recent`

适合：

- 排查最近事件
- 调试过滤表达式
- 运维观察

### 10.2 `/api/hook/stream`

适合：

- 实时消费
- 调试前端或外部观察程序
- 对接轻量事件订阅方

### 10.3 插件

适合：

- 同进程业务接入
- 需要更复杂处理逻辑
- 需要将事件转发到第三方系统

三者的事件源一致，过滤语义一致，只是使用方式不同。

## 11. 推荐使用方式

### 11.1 业务和 `lalmax` 同进程

优先用插件：

- 延迟低
- 无需再走 HTTP
- 易于封装业务逻辑

### 11.2 业务和 `lalmax` 不同进程

优先用：

- `/api/hook/stream`
- 或内置 HTTP notify 插件

### 11.3 需要统一平台出口

可以继续在插件层增加：

- Kafka 插件
- Redis 插件
- 数据库存档插件

## 12. 当前边界与限制

### 12.1 `app_name` 边界

当前上游 `lal` 的 `WithOnHookSession` 仍只提供 `streamName`，不提供 `appName`。

这意味着：

- `lal` 原生 group 状态本身是可信的
- 但扩展订阅者与 `app_name` 的精确归属能力仍受上游输入限制

因此文档里一直建议：

- 需要精确路由时，尽量同时使用 `app_name + stream_name`

### 12.2 插件可靠性策略

当前插件队列满时是丢弃策略，不是阻塞策略，也不是持久化重试策略。

这是为了媒体主链路稳定。

如果未来某类插件需要强可靠投递，建议不要直接在 `lalmax` 内核层强推重试，而是：

- 插件内自己做持久化
- 或者把事件转发给外部消息系统

### 12.3 当前插件装配方式

目前插件仍然通过代码注册。

也就是说：

- 你需要拿到 `LalMaxServer`
- 调用 `RegisterHookPlugin(...)`

下一步可以继续演进成“配置化装配”，由配置声明启用哪些插件和参数。

## 13. 后续演进建议

比较合理的后续方向有 3 个。

### 13.1 插件配置化装配

目标：

- 不用业务代码手动注册插件
- 配置文件直接声明插件列表、参数、过滤条件

### 13.2 标准化插件参数

例如统一定义：

- HTTP webhook 插件参数
- Kafka 插件参数
- Redis 插件参数

### 13.3 更强的可靠性模型

例如：

- 插件失败重试
- 死信队列
- 插件级别熔断
- 指标与监控

## 14. 小结

现在的 Hook 架构已经完成了从“固定 HTTP 回调实现”到“统一 HookHub + 插件化业务处理”的转换。

当前职责边界可以概括为：

- `lal`: 原生媒体事件事实源
- `lalmax` HookHub: 聚合、过滤、缓存、分发
- 插件: 具体业务处理

这套结构的核心价值是：

- 主流程稳定
- 业务接入灵活
- 多业务可并存
- 后续扩展成本更低
