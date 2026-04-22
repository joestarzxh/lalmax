# Hook 与流管理改造分析

本文档分析两个问题：

- `hook` 是否适合改成类似 lal `logic` 的管理方式。
- lal 中所有流信息是否适合都由 lalmax 来管理。

结论先行：

- `hook` 适合改造成“lalmax 扩展流管理层”，但不适合照搬 lal `logic.Group` 的完整职责。
- lal 原生流生命周期、原生协议会话、remux、GOP 缓存、统计计算，仍应由 lal `logic` 作为事实源。
- lalmax 更适合作为“扩展能力管理者 + 统一查询门面”，聚合 lal 原生状态和 lalmax 扩展订阅状态，而不是接管 lal 的全部流状态。

## 当前职责边界

### lal `logic`

lal `logic` 当前承担流媒体内核职责：

- 管理 `Group` 生命周期。
- 管理输入流：RTMP pub、RTSP pub、GB28181/RTP PS pub、自定义 pub、relay pull。
- 管理输出流：RTMP sub、RTSP sub、HTTP-FLV sub、HTTP-TS sub、HLS sub、relay push。
- 管理协议转换链路：RTMP 到 RTSP、RTMP 到 MPEG-TS、RTSP/PS 到 RTMP 等。
- 管理 GOP 缓存、HLS、录制、转推、回源。
- 维护 `StatGroup`、`StatSession`、码率、在线状态、HTTP API 状态。
- 处理输入流生命周期中的 hook 回调：`WithOnHookSession`。

lal 的 `Group` 是强状态对象，核心特征是：

- 单流强一致生命周期。
- 所有核心协议会话在同一个锁内增删。
- 统计信息从真实 session 对象计算。
- 输入流和输出流互相关联，任何重复管理都会产生状态一致性问题。

### lalmax `hook`

lalmax 当前 `hook` 是 lal `WithOnHookSession` 的业务扩展层：

- lal 输入流出现时创建 `HookSession`。
- 接收 lal 分发出来的 RTMP 消息。
- 自己维护一份轻量 GOP 缓存。
- 给 WHEP、Jessibuca、HTTP-FMP4、HLS-FMP4/LLHLS 等 lalmax 扩展消费者分发数据。
- 给新消费者回放缓存 GOP。
- 在 lalmax HTTP API 里补充扩展消费者统计。

当前 `hook` 的特点：

- 管理粒度只有 `streamName`，没有 `appName`。
- `HookSessionMangaer` 是全局 singleton + `sync.Map`。
- `consumerInfo` 的 `StatSession` 很轻，协议、远端地址、读写字节、码率等大多未完整填充。
- 扩展消费者生命周期主要由各模块自行调用 `AddConsumer` / `RemoveConsumer`。
- `hook` 不管理 lal 原生协议 session。

这说明 `hook` 当前不是流内核，而是 lalmax 扩展订阅层。

## 是否适合改成 lal `logic` 那样的管理

适合借鉴，不适合照搬。

### 适合借鉴的部分

`hook` 可以借鉴 lal `logic` 的管理模型，形成更清晰的 lalmax 扩展流管理层：

- 抽象 `HookManager` 接口，避免全局 singleton 直接散落使用。
- 抽象 `HookGroup` 或 `ExtGroup`，按流管理扩展消费者。
- 支持 `GetOrCreateGroup`、`GetGroup`、`Iterate`、`Len` 等方法。
- 支持 `appName + streamName` 的流标识，为未来复杂路径做准备。
- 将扩展消费者按类型管理，例如 WHEP、Jessibuca、HTTP-FMP4、HLS-FMP4。
- 统一扩展消费者的 `StatSession` 填充、码率更新、远端地址、协议名和关闭逻辑。
- 把 `GetAllConsumer()` 做成可信统计，而不是临时拼接。
- 增加统一的 `Tick` 或定时统计，避免扩展订阅没有码率。
- 用明确的生命周期事件：`OnLalInputStart`、`OnRtmpMsg`、`OnLalInputStop`、`AddExtSub`、`DelExtSub`。

这些改造能解决当前 `hook` 的几个问题：

- 统计不准。
- 生命周期分散。
- 扩展订阅者和 lal 原生订阅者没有统一视图。
- 未来新增扩展协议时容易继续堆在 `HookSession` 里。
- `streamName` 作为唯一 key 对多 appName 场景不友好。

### 不适合照搬的部分

不建议把 lal `logic.Group` 的职责完整搬到 `hook`：

- lal `Group` 已经负责协议转发、remux、缓存、录制、推拉流生命周期。
- hook 只能拿到 lal 分发后的 RTMP 消息，不拥有原生输入 session。
- 如果 hook 也管理原生 pub/sub/pull，会和 lal 内部 `Group` 形成双事实源。
- 双事实源会带来状态不一致，例如 lal 内部 session 已关闭但 lalmax 仍认为在线。
- lal 升级后内部 session 行为变化，lalmax 复制逻辑很容易漂移。
- hook 回调在 lal 内部处理链路上，同步阻塞会影响 lal 核心转发性能。

因此，合理目标不是“把 hook 改成另一个 lal logic”，而是“把 hook 改成 lalmax 扩展订阅的 logic”。

## lal 所有流信息是否应由 lalmax 管理

不建议。

### 不适合全部由 lalmax 管理的原因

lal 原生流信息应该继续由 lal 管理，原因是：

- lal 才拥有真实的原生 session 对象。
- lal 才知道 RTMP、RTSP、HTTP-FLV、HTTP-TS、HLS-TS、pull、push 的真实生命周期。
- lal 统计来自真实连接读写字节，lalmax 外层无法无损重建。
- lal 内部 group 还管理 remuxer、GOP 缓存、HLS muxer、录制、回源、转推。
- lalmax 如果接管这些状态，要么侵入 lal 内部，要么复制大量逻辑。
- 复制后会产生“lal 状态”和“lalmax 状态”不一致的问题。

典型风险：

- 拉流 session 已经断开，lalmax 未收到对应事件。
- lal 内部因错误关闭 group，lalmax 仍保留扩展状态。
- 同名 `streamName` 在不同 `appName` 下冲突。
- API 返回的 pub/sub 数量与真实 lal 内部不一致。
- 后续升级 lal 版本时，lalmax 需要同步内部结构变化。

### 适合由 lalmax 管理的内容

lalmax 适合管理以下内容：

- lalmax 扩展协议服务：SRT、WHIP、WHEP、Jessibuca、HTTP-FMP4、HLS-FMP4/LLHLS、GB28181 控制面。
- 扩展消费者生命周期和统计。
- 扩展协议相关的缓存、队列、写超时、回放策略。
- 对外统一 HTTP API 聚合视图。
- lal 原生状态的只读镜像或快照。

也就是说，lalmax 应该做：

- `lal.StatAllGroup()` 的聚合增强。
- lalmax 扩展订阅状态补充。
- 统一 API 输出。
- 统一控制入口转发给 lal 或 lalmax 对应模块。

lalmax 不应该做：

- 替代 lal 管理 RTMP/RTSP/HTTP-FLV/HTTP-TS/HLS-TS session。
- 复制 lal `Group` 内部 remux 和协议状态机。
- 自己维护一份“原生 pub/sub/pull 是否存在”的权威状态。

## 推荐架构

推荐采用“双层管理，单一事实源”的架构。

> 落地说明：第一阶段已将原 `hook` 包迁移为 `lalmax/logic` 扩展管理包。为了避免和上游 `github.com/q191201771/lal/pkg/logic` 导入名冲突，调用侧通常使用 `maxlogic` 作为别名。

### 第一层：lal 原生事实源

lal `logic` 继续管理：

- 原生输入输出 session。
- 原生 group。
- 原生统计。
- 原生控制 API。
- 原生协议转发。

lalmax 通过 `ILalServer` 查询：

- `StatLalInfo`
- `StatAllGroup`
- `StatGroup`
- `CtrlStartRelayPull`
- `CtrlStopRelayPull`
- `CtrlKickSession`

### 第二层：lalmax 扩展管理层

新增或重构 `hook` 为扩展流管理层，例如：

```text
LalMaxStreamManager
  HookGroup(streamKey)
    input snapshot
    GOP cache for extension protocols
    extension subscribers
      WHEP
      Jessibuca
      HTTP-FMP4
      HLS-FMP4/LLHLS
    extension stats
```

这里的 `HookGroup` 只管理 lalmax 扩展状态，不接管 lal 原生状态。

### 第三层：统一 API 门面

lalmax HTTP API 输出时：

```text
lal stat group
  + lalmax extension subscribers
  + lalmax extension module status
  = unified stat response
```

这样对外看起来是 lalmax 管理了全部视图，但内部事实源仍然清晰。

## Hook 管理层建议设计

### 流标识

建议引入显式 `StreamKey`：

```text
StreamKey {
  AppName
  StreamName
}
```

短期可继续兼容只有 `streamName` 的模式：

- `AppName` 为空时，按 `streamName` 匹配。
- 未来如果启用 lal `ComplexGroupManager`，可以平滑过渡。

### Manager 接口

建议从全局 singleton 收敛为可注入对象：

```text
type Manager interface {
  OnInputStart(key, uniqueKey)
  OnInputMsg(key, msg)
  OnInputStop(key)
  AddSubscriber(key, sub)
  RemoveSubscriber(key, subscriberID)
  GetGroup(key)
  StatGroups()
}
```

第一阶段已经改为 `GetGroupManagerInstance()`，不再保留旧的 `GetHookSessionManagerInstance()` 入口；后续如果要进一步降低全局状态依赖，可再将 manager 挂到 `LalMaxServer` 实例上。

### Group 职责

`HookGroup` 建议只做：

- 保存输入流元信息。
- 保存音视频头。
- 保存扩展协议所需 GOP cache。
- 管理扩展订阅者。
- 管理扩展订阅者回放顺序。
- 统计扩展订阅者信息。
- 在输入流停止时通知扩展订阅者。

不建议做：

- 管理 lal 原生 pub/sub/pull session。
- 参与 lal 原生 remux 链路。
- 替代 lal 原生 GOP cache。

### Subscriber 接口

当前 `IHookSessionSubscriber` 过于简单：

```text
OnMsg(msg)
OnStop()
```

建议扩展为更完整的订阅者描述：

```text
Subscriber {
  ID
  Protocol
  RemoteAddr
  StartTime
  ReplayPolicy
  OnMsg
  OnStop
  UpdateStat
  Stat
}
```

这样 `GetAllConsumer()` 才能返回可靠统计，而不是只返回 `SessionId` 和 `StartTime`。

## 迁移方案

### 阶段一：保持事实源不变，整理 hook

目标：

- 不改 lal 交互方式。
- 不改扩展协议行为。
- 只重构 hook 内部管理模型。

内容：

- 新增 `lalmax/logic` 扩展管理包，避免继续使用 `hook` 作为业务层命名。
- 使用 `Group`、`IGroupManager`、`ComplexGroupManager`、`Subscriber`、`ReplaySubscriber` 等更接近 lal `logic` 的命名。
- 引入 `StreamKey{AppName, StreamName}`，管理器支持 `appName + streamName` 精确匹配。
- 保留 `streamName` 单键兼容查找；当空 `appName` 查到多个同名不同 appName 的流时返回未命中，避免随机匹配。
- 将 `HookSession` 职责迁移到扩展 `Group`，只管理扩展 GOP 缓存和扩展订阅者。
- 完善扩展 subscriber stat。
- 保留现有 `AddConsumer` 兼容方法。
- WHEP、Jessibuca、HTTP-FMP4、HLS-FMP4、stat group 支持可选 `app_name` 参数。
- lal 当前 `WithOnHookSession` 只传 `uniqueKey` 和 `streamName`，所以 lal 原生回调创建的扩展 `Group` 仍默认 `AppName` 为空；后续如果 lal 回调能提供 appName，可直接改为 `NewGroup(uniqueKey, StreamKey{AppName, StreamName}, ...)`。

收益：

- 风险低。
- 统计更准确。
- 后续新增扩展协议更清晰。

### 阶段二：lalmax API 做聚合视图

目标：

- lal 仍是原生事实源。
- lalmax 输出统一完整状态。

内容：

- `statGroupHandler` 从 lal 获取原生 `StatGroup`。
- 从 lalmax manager 获取扩展订阅者。
- 合并到响应中。
- 为扩展订阅者设置明确协议名，例如 `WHEP`、`JESSIBUCA`、`FMP4`、`LLHLS`。

收益：

- 对外统一。
- 内部边界清晰。

### 阶段三：事件化和异步化

目标：

- 降低 hook 对 lal 核心链路的阻塞风险。

内容：

- `OnMsg` 内部尽量只做轻量复制和入队。
- 扩展分发在独立 goroutine 中完成。
- 每个订阅者有明确队列上限和丢弃/断开策略。
- 慢消费者不会拖住 lal 原生转发。

收益：

- 更适合多扩展订阅者和慢客户端。
- 避免 WHEP/FMP4 等写阻塞影响原始流。

### 阶段四：按需考虑 appName 复杂管理

目标：

- 支持多 appName 同 streamName。

内容：

- 跟随 lal `ComplexGroupManager` 的语义。
- 统一流 key 解析规则。
- API 支持 `app_name` 可选参数。

收益：

- 适合更复杂的多租户或多应用路径。

## 不推荐方案

### 方案：lalmax 接管 lal 全部流状态

不推荐，除非准备 fork lal 并长期维护。

问题：

- 会复制 lal 内部逻辑。
- 会破坏 lal 作为内核库的边界。
- 会增加升级 lal 的成本。
- 会引入双事实源一致性问题。

### 方案：hook 直接改成 lal `Group` 等价物

不推荐。

问题：

- hook 没有原生 session 对象。
- hook 只接收已经 remux 成 RTMP 的消息。
- 管理范围和 lal `Group` 不对等。
- 容易形成“看起来像 logic，但不具备 logic 真实能力”的半成品。

## 建议结论

推荐方向：

- lal 保持流媒体内核和原生状态事实源。
- lalmax 增加自己的扩展流管理层，借鉴 lal `logic` 的 manager/group/session 模型。
- lalmax API 做统一聚合视图，而不是把 lal 的状态搬出来重新管理。
- hook 改造重点放在扩展订阅者生命周期、统计、队列、回放和慢消费者隔离。

一句话：`hook` 可以“logic 化”，但应该是 lalmax 扩展层的 logic，而不是替代 lal 的 logic。
