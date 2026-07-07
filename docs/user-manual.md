# easy_asr 用户手册

## 1. 工具定位

`easy_asr` 是一个 Go/Cobra CLI，用于把本地录音文件转换为 SRT 字幕。它面向两类使用者：

- 人：命令短、默认行为清楚、错误信息可读。
- Agent / 脚本：stdout 稳定、支持 `--json`、错误走 stderr、exit code 可判断。

DashScope engine 链路：

1. 读取本地音频文件。
2. 上传到配置中的对象存储。
3. 生成临时可访问 URL。
4. 调用所选 DashScope ASR engine 的异步转写任务。
5. 轮询任务直到完成或超时。
6. 下载转写 JSON。
7. 渲染 SRT。
8. 默认清理临时对象。

MiMo engine 链路：

1. 读取本地音频文件。
2. 用 `ffprobe` 获取总时长。
3. 用 `ffmpeg` 归一化为 16 kHz mono WAV。
4. 用 Silero VAD v6 + ONNX Runtime 寻找分段断点。
5. 切成约 180 秒的 WAV segment。
6. 把每段作为 `data:audio/wav;base64,...` 调用 MiMo API。
7. 合并 raw JSON wrapper 并渲染按原始时间轴排列的 SRT。

## 2. 命令总览

```zsh
easy_asr --help
```

主要子命令：

- `transcribe`：转写音频并生成 SRT。
- `config path`：输出配置文件路径。
- `config validate`：验证配置是否完整。
- `config init`：创建默认配置模板，不会覆盖已有配置。
- `engines`：列出已注册 engine。
- `assets install`：下载 Silero VAD v6 ONNX 模型到用户缓存目录。
- `doctor`：检查 ffmpeg、ffprobe、Silero 模型和 ONNX Runtime。
- `schema run-result`：输出 `--json` 结果结构说明。

## 3. 转写音频

### 3.1 默认转写

```zsh
easy_asr transcribe input.mp3
```

默认输出：

- 生成 `input.srt`。
- stdout 输出 `input.srt` 的绝对路径。
- 临时对象存储文件会在任务结束后删除。

### 3.2 指定输出路径

```zsh
easy_asr transcribe input.mp3 -o /tmp/output.srt
```

### 3.3 保存原始 JSON

```zsh
easy_asr transcribe input.mp3 \
  -o /tmp/output.srt \
  --raw-json /tmp/output.raw.json
```

原始 JSON 适合后续做摘要、分段分析、说话人处理或自定义字幕渲染。

### 3.4 机器可读输出

```zsh
easy_asr transcribe input.mp3 --json
```

示例结构：

```json
{
  "engine": "qwen3-asr-flash-filetrans",
  "task_id": "task-id",
  "output_path": "/absolute/path/input.srt",
  "raw_json_path": "",
  "object_key": "easy_asr/tmp/20260630/example/input.mp3",
  "transcription_url": "https://example/result.json?<redacted>",
  "usage_seconds": 29
}
```

注意：`transcription_url` 的 query 会脱敏，避免把 signed URL 泄露到日志或 Agent 上下文。

### 3.5 输出 SRT 到 stdout

```zsh
easy_asr transcribe input.mp3 --stdout
```

这个模式适合管道处理：

```zsh
easy_asr transcribe input.mp3 --stdout > input.srt
```

## 4. 常用参数

```text
--engine string              指定 ASR engine，默认 qwen3-asr-flash-filetrans
-o, --output string          指定 SRT 输出路径
--raw-json string            保存原始转写 JSON
--json                       stdout 输出机器可读运行结果
--stdout                     stdout 输出 SRT 内容
--keep-object                保留临时对象存储文件
--language string            语言提示，例如 zh
--channel ints               音频 channel index，可重复
--hotwords string            热词文本
--hotwords-file string       从文件读取热词
--vocabulary-id string       Fun-ASR 热词表 ID
--no-diarization             关闭 Fun-ASR 说话人分离
--speaker-count int          Fun-ASR 说话人数量参考值，范围 2 到 100
--enable-itn                 启用 inverse text normalization
--enable-words               启用词级时间戳，默认开启
--no-enable-words            关闭词级时间戳
--poll-interval duration     轮询间隔，默认 5s
--poll-timeout duration      轮询超时，默认 2h
--request-timeout duration   HTTP 请求超时，默认 30s
--config string              指定配置文件路径
```

MiMo 不支持 DashScope/Fun-ASR 专属参数；显式传入 `--hotwords`、`--hotwords-file`、`--vocabulary-id`、`--channel`、`--no-diarization`、`--speaker-count`、`--enable-itn`、`--enable-words` 或 `--no-enable-words` 会返回 usage error。

## 5. 配置

默认配置路径：

```text
~/.config/easy_asr/config.yaml
```

查看路径：

```zsh
easy_asr config path
```

验证配置：

```zsh
easy_asr config validate
```

创建模板：

```zsh
easy_asr config init
```

`config init` 不会覆盖已有配置。如果配置已存在，会返回使用错误。

### 5.1 配置结构

配置文件使用 YAML。敏感值不要提交到 git。

```yaml
engine: qwen3-asr-flash-filetrans
engines:
  qwen3_asr_flash_filetrans:
    dashscope:
      api_key: "<dashscope-api-key>"
      base_url: "https://dashscope.aliyuncs.com/api/v1"
      model: "qwen3-asr-flash-filetrans"
    oss:
      region: "cn-beijing"
      endpoint: "https://oss-cn-beijing.aliyuncs.com"
      bucket: "<bucket>"
      access_key_id: "<access-key-id>"
      access_key_secret: "<access-key-secret>"
      key_prefix: "easy_asr/tmp"
      presign_ttl: "24h0m0s"
    asr:
      channel_ids: [0]
      enable_itn: false
      enable_words: true
      poll_interval: "5s"
      poll_timeout: "2h0m0s"
      request_timeout: "30s"
  fun_asr:
    dashscope:
      model: "fun-asr"
    asr:
      diarization_enabled: true
  mimo_v2_5_asr:
    mimo:
      api_key: "<mimo-api-key>"
      base_url: "https://api.xiaomimimo.com/v1"
      model: "mimo-v2.5-asr"
    asr:
      language: "auto"
      request_timeout: "5m0s"
    segmentation:
      target_duration: "3m0s"
      min_duration: "2m30s"
      max_duration: "3m30s"
      vad_threshold: 0.5
      min_silence: "100ms"
      speech_pad: "30ms"
```

`fun_asr` 默认继承 `qwen3_asr_flash_filetrans` 下的 DashScope API key、`base_url`、对象存储配置和轮询配置。只需要在 `fun_asr` 中写差异项，例如改模型、热词表 ID 或关闭说话人分离。

当前本机配置已按可用对象存储和 DashScope key 填好。

### 5.2 环境变量覆盖

配置可被环境变量覆盖，常用变量：

```text
EASY_ASR_ENGINE
EASY_ASR_DASHSCOPE_API_KEY
DASHSCOPE_API_KEY
ALIBABA_DASHSCOPE_API_KEY
EASY_ASR_DASHSCOPE_BASE_URL
EASY_ASR_DASHSCOPE_MODEL
EASY_ASR_OSS_REGION
EASY_ASR_OSS_ENDPOINT
AWS_S3_ENDPOINT_URL
EASY_ASR_OSS_BUCKET
AWS_STORAGE_BUCKET_NAME
EASY_ASR_OSS_ACCESS_KEY_ID
AWS_ACCESS_KEY_ID
EASY_ASR_OSS_ACCESS_KEY_SECRET
AWS_SECRET_ACCESS_KEY
EASY_ASR_OSS_KEY_PREFIX
EASY_ASR_MIMO_API_KEY
MIMO_API_KEY
EASY_ASR_MIMO_BASE_URL
EASY_ASR_MIMO_MODEL
EASY_ASR_SILERO_VAD_MODEL
EASY_ASR_ONNXRUNTIME_LIBRARY
```

## 6. Engine 设计

查看 engine：

```zsh
easy_asr engines
easy_asr engines --json
```

当前状态：

- `qwen3-asr-flash-filetrans`：已实现，默认。
- `fun-asr`：已实现，DashScope 异步 Fun-ASR 录音文件识别。

如果指定未知 engine，CLI 会返回 usage error。

### 6.1 Fun-ASR 参数

Fun-ASR 支持：

- `--language zh`：映射到 `language_hints`。
- `--channel 0`：映射到 `channel_id`。
- `--vocabulary-id <id>`：使用百炼控制台创建的热词表。
- `--speaker-count 2`：给说话人分离提供人数参考。
- `--no-diarization`：关闭默认启用的说话人分离。

Fun-ASR 不支持 Qwen3 专属的 `--hotwords`、`--hotwords-file`、`--enable-itn`、`--enable-words` 和 `--no-enable-words`。传入这些参数时 CLI 会返回 usage error。

说话人分离仅适用于单声道或单个 channel。默认开启说话人分离时，如果传入多个 `--channel`，CLI 会在本地返回 usage error；如需多 channel 转写，请显式传入 `--no-diarization`。

## 7. 输出契约

### 7.1 人类默认模式

```zsh
easy_asr transcribe input.mp3
```

成功时 stdout 只有一行：

```text
/absolute/path/input.srt
```

这样既适合人复制，也适合脚本捕获。

### 7.2 Agent 模式

```zsh
easy_asr transcribe input.mp3 --json
```

stdout 是稳定 JSON；stderr 承载错误和诊断。Agent 不需要解析自然语言进度。

### 7.3 Exit code

- `0`：成功。
- `1`：运行时错误，例如网络错误、ASR 任务失败、对象存储失败。
- `2`：用法或配置错误，例如参数缺失、engine 未实现、配置缺字段。

## 8. 文件安全

- 配置目录应为 `0700`。
- 配置文件应为 `0600`。
- CLI 不会通过 config 命令打印完整 secret。
- 输出中的 signed URL query 会脱敏。
- 默认会清理上传到对象存储的临时音频。

如需保留对象用于排查：

```zsh
easy_asr transcribe input.mp3 --keep-object --json
```

排查完成后需要手动清理对象。

## 9. 排查

### 9.1 配置不完整

```zsh
easy_asr config validate
```

如果提示缺少 `dashscope.api_key`、`oss.bucket` 等字段，检查 `~/.config/easy_asr/config.yaml` 或对应环境变量。

### 9.2 ASR 任务失败

使用 `--json` 保留 task id：

```zsh
easy_asr transcribe input.mp3 --json
```

常见原因：

- 对象存储 signed URL 不可被 DashScope 访问。
- 音频格式或内容不符合模型要求。
- DashScope API key 无权限或额度不足。
- 网络超时。

### 9.3 想保留更多诊断材料

```zsh
easy_asr transcribe input.mp3 \
  -o /tmp/debug.srt \
  --raw-json /tmp/debug.raw.json \
  --keep-object \
  --json
```

注意：`--keep-object` 会留下临时音频对象，排查结束后应清理。

## 10. 开发验证命令

在项目根目录运行：

```zsh
go test ./...
go test -race ./...
go vet ./...
go build -o bin/easy_asr ./cmd/easy_asr
```

安装到本机用户 bin：

```zsh
go build -o ~/.local/bin/easy_asr ./cmd/easy_asr
```
