# lal 原生配置说明

本文档说明 `conf/lalmax.conf.json` 中 `lal` 配置段的常用字段。`lal` 配置段会直接传给 lal 原生服务，用于 RTMP、RTSP、HTTP-FLV、HLS-TS、HTTP-TS、录制、鉴权和原生 HTTP API。

## rtmp

- `enable`: 是否启用 RTMP 服务。
- `addr`: RTMP 监听地址，例如 `:1935`。
- `rtmps_enable`: 是否启用 RTMPS。
- `rtmps_addr`: RTMPS 监听地址，例如 `:4935`。
- `rtmps_cert_file`: RTMPS 证书文件路径。
- `rtmps_key_file`: RTMPS 私钥文件路径。
- `gop_num`: RTMP 拉流 GOP 缓存数量。
- `single_gop_max_frame_num`: 单个 GOP 最大缓存帧数，`0` 表示不限制。
- `merge_write_size`: 合并写大小，`0` 表示关闭合并写。

## in_session

- `add_dummy_audio_enable`: 没有音频时是否补静音音频。
- `add_dummy_audio_wait_audio_ms`: 等待真实音频的时间，超过后才补静音音频。

## default_http

HTTP 类协议的默认监听配置。HTTP-FLV、HTTP-TS、HLS-TS 未单独配置监听地址时，会使用这里的地址。

- `http_listen_addr`: 默认 HTTP 监听地址，例如 `:8080`。
- `https_listen_addr`: 默认 HTTPS 监听地址，例如 `:4433`。
- `https_cert_file`: HTTPS 证书文件路径。
- `https_key_file`: HTTPS 私钥文件路径。

## httpflv

- `enable`: 是否启用 HTTP-FLV。
- `enable_https`: 是否启用 HTTPS HTTP-FLV。
- `url_pattern`: URL 路径匹配前缀。示例配置为 `/`，因此 `/live/test110.flv` 可用。
- `gop_num`: HTTP-FLV GOP 缓存数量。
- `single_gop_max_frame_num`: 单个 GOP 最大缓存帧数。

## hls

这里是 lal 原生 HLS-TS 配置，不是 lalmax 的 HLS-FMP4/LLHLS 配置。

- `enable`: 是否启用 HLS-TS。
- `enable_https`: 是否启用 HTTPS HLS-TS。
- `url_pattern`: HLS-TS URL 路径前缀，常用 `/hls/`。
- `out_path`: HLS-TS 文件输出目录。
- `fragment_duration_ms`: 单个 TS 分片时长。
- `fragment_num`: m3u8 中保留的分片数量。
- `delete_threshold`: 清理旧分片的阈值。
- `cleanup_mode`: 清理模式。
- `use_memory_as_disk_flag`: 是否使用内存模拟磁盘。
- `sub_session_timeout_ms`: HLS 拉流会话超时时间。
- `sub_session_hash_key`: HLS 会话哈希 key。

## httpts

- `enable`: 是否启用 HTTP-TS。
- `enable_https`: 是否启用 HTTPS HTTP-TS。
- `url_pattern`: URL 路径匹配前缀。示例配置为 `/`，因此 `/live/test110.ts` 可用。
- `gop_num`: HTTP-TS GOP 缓存数量。
- `single_gop_max_frame_num`: 单个 GOP 最大缓存帧数。

## rtsp

- `enable`: 是否启用 RTSP。
- `addr`: RTSP 监听地址，例如 `:5544`。
- `rtsps_enable`: 是否启用 RTSPS。
- `rtsps_addr`: RTSPS 监听地址，例如 `:5322`。
- `rtsps_cert_file`: RTSPS 证书文件路径。
- `rtsps_key_file`: RTSPS 私钥文件路径。
- `out_wait_key_frame_flag`: RTSP 拉流是否等待关键帧后再输出。
- `auth_enable`: 是否启用 RTSP 鉴权。
- `auth_method`: 鉴权方式。
- `username`: RTSP 鉴权用户名。
- `password`: RTSP 鉴权密码。

## record

- `enable_flv`: 是否启用 FLV 录制。
- `flv_out_path`: FLV 录制输出目录。
- `enable_mpegts`: 是否启用 MPEG-TS 录制。
- `mpegts_out_path`: MPEG-TS 录制输出目录。

## relay_push

- `enable`: 是否启用静态转推。
- `addr_list`: 转推目标地址列表。

## static_relay_pull

- `enable`: 是否启用静态回源拉流。
- `addr`: 静态回源地址。

## http_api

- `enable`: 是否启用 lal 原生 HTTP API。
- `addr`: lal 原生 HTTP API 监听地址，例如 `:8083`。

接口说明见 [lal_api.md](./lal_api.md)。

## simple_auth

简单鉴权配置，鉴权值通常按 `key + streamName` 计算。

- `key`: 鉴权 key。
- `dangerous_lal_secret`: 管理类接口使用的 secret。
- `pub_rtmp_enable`: 是否启用 RTMP 推流鉴权。
- `sub_rtmp_enable`: 是否启用 RTMP 拉流鉴权。
- `sub_httpflv_enable`: 是否启用 HTTP-FLV 拉流鉴权。
- `sub_httpts_enable`: 是否启用 HTTP-TS 拉流鉴权。
- `pub_rtsp_enable`: 是否启用 RTSP 推流鉴权。
- `sub_rtsp_enable`: 是否启用 RTSP 拉流鉴权。
- `hls_m3u8_enable`: 是否启用 HLS m3u8 鉴权。

## pprof

- `enable`: 是否启用 pprof。
- `addr`: pprof 监听地址，例如 `:8084`。

## log

- `level`: 日志级别。
- `filename`: 日志文件路径。
- `is_to_stdout`: 是否输出到标准输出。
- `is_rotate_daily`: 是否按天切分日志。
- `short_file_flag`: 是否打印短文件名。
- `timestamp_flag`: 是否打印时间戳。
- `timestamp_with_ms_flag`: 时间戳是否包含毫秒。
- `level_flag`: 是否打印日志级别。
- `assert_behavior`: 断言行为。

## debug

- `log_group_interval_sec`: group 状态日志输出间隔。
- `log_group_max_group_num`: 单次最多输出的 group 数量。
- `log_group_max_sub_num_per_group`: 单个 group 最多输出的订阅者数量。
