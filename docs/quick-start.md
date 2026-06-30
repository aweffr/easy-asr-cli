# easy_asr 快速开始

`easy_asr` 是本机音频转字幕 CLI，用于把本地音频文件上传到对象存储后调用 ASR 引擎转写，并生成 `.srt` 字幕文件。

当前默认并已实现的引擎是 `qwen3-asr-flash-filetrans`。`fun-asr` 和 `seed-asr` 已预留为后续扩展入口，本期不会调用。

## 安装位置

如果使用源码构建：

```zsh
go build -o bin/easy_asr ./cmd/easy_asr
```

如果已经把二进制放入 `PATH`，可以直接使用：

```zsh
easy_asr --help
```

## 配置文件

默认配置文件已填好：

```text
~/.config/easy_asr/config.yaml
```

权限应保持为：

```zsh
stat -f '%Sp %N' ~/.config/easy_asr ~/.config/easy_asr/config.yaml
```

预期结果：

```text
drwx------ .../.config/easy_asr
-rw------- .../.config/easy_asr/config.yaml
```

验证配置：

```zsh
easy_asr config validate
```

输出 `ok` 表示可用。

## 最常用命令

把 `input.mp3` 转成同目录同名 `.srt`：

```zsh
easy_asr transcribe input.mp3
```

命令成功时，stdout 只输出生成的字幕文件绝对路径，方便脚本或 Agent 读取。

指定输出文件：

```zsh
easy_asr transcribe input.mp3 -o output.srt
```

同时保存原始 ASR JSON：

```zsh
easy_asr transcribe input.mp3 -o output.srt --raw-json output.raw.json
```

输出机器可读 JSON：

```zsh
easy_asr transcribe input.mp3 --json
```

直接把 SRT 内容写到 stdout：

```zsh
easy_asr transcribe input.mp3 --stdout
```

## Agent 友好用法

推荐 Agent 使用 `--json`：

```zsh
easy_asr transcribe /absolute/path/audio.mp3 --json
```

返回结果包含：

- `engine`：实际使用的 ASR engine
- `task_id`：DashScope task id
- `output_path`：SRT 文件路径
- `raw_json_path`：原始 JSON 路径，未指定时为空
- `object_key`：临时对象存储 key
- `transcription_url`：结果 URL，query 已脱敏
- `usage_seconds`：ASR 计费/处理时长
- `cleanup_error`：临时对象清理失败时才出现

默认模式下：

- stdout：只输出结果路径或 JSON
- stderr：输出进度、诊断和错误
- exit code `0`：成功
- exit code `2`：参数或配置错误
- exit code `1`：运行时错误

## 端到端检查

可以用本机已有音频做烟测：

```zsh
easy_asr transcribe \
  /path/to/sample.mp3 \
  -o /tmp/easy_asr_smoke.srt \
  --raw-json /tmp/easy_asr_smoke.raw.json \
  --json
```

检查字幕预览：

```zsh
sed -n '1,40p' /tmp/easy_asr_smoke.srt
```
