# go-tts / VoxLab

一个使用 Go + [`go-web-frame`](https://github.com/chuccp/go-web-frame) +
`html/template` 实现的多引擎文字转语音 Web 应用，支持：

- **Kokoro**：本地 ONNX、`espeak-ng` 音素化、多语言音色、语速控制。
- **Kitten TTS**：本地轻量 ONNX、8 个内置音色、语速控制。
- **Edge TTS**：微软 Edge Read Aloud 在线音色，支持 400+ 音色和 90+ 语言。
- 同一个页面内切换引擎、选择音色、调节语速、试听和下载音频。
- 本地模型缺失或初始化失败时，仅禁用对应引擎，不影响 Edge TTS 使用。

Kokoro 的 Go 接入方案参考了
[chasedputnam/go-kokoro-tts](https://github.com/chasedputnam/go-kokoro-tts)；
Kokoro 与 Kitten 共用一份 ONNX Runtime 环境。

## 系统要求

- Go 1.26 或更高版本，且启用 CGO。
- C 编译器：Windows 可使用 MinGW-w64，macOS 使用 Xcode Command Line Tools，Linux 使用 GCC。
- `espeak-ng`：Kokoro 和 Kitten 的音素化依赖。
- ONNX Runtime **1.25.0**：`onnxruntime_go` 的 C API 头文件对应该版本。
- 网络：仅 Edge TTS 需要联网。

Windows 可下载
[ONNX Runtime 1.25.0](https://github.com/microsoft/onnxruntime/releases/tag/v1.25.0)
中的 `onnxruntime-win-x64-1.25.0.zip`。解压后把 `lib/onnxruntime.dll` 放到项目根目录、
`runtime/` 或 `lib/` 下，也可以使用 `-onnx-lib` 指定绝对路径。

`espeak-ng` 下载：

- Windows：[espeak-ng 1.52.0](https://github.com/espeak-ng/espeak-ng/releases/tag/1.52.0)
- macOS：`brew install espeak-ng`
- Ubuntu/Debian：`sudo apt install espeak-ng`

## 下载本地模型

### Kokoro

```text
models/
  kokoro/
    kokoro-v0_19.onnx
    voices-v1.0.bin
```

- 模型：[kokoro-v0_19.onnx](https://github.com/nazdridoy/kokoro-tts/releases/download/v1.0.0/kokoro-v0_19.onnx)
- 音色：[voices-v1.0.bin](https://github.com/nazdridoy/kokoro-tts/releases/download/v1.0.0/voices-v1.0.bin)

虽然音色文件扩展名是 `.bin`，实际是包含多个 `.npy` 的 NPZ。应用也支持包含多个
`.npz`、`.npy` 或 raw float32 `.bin` 文件的音色目录。

### Kitten TTS

```text
models/
  kitten/
    kitten_tts_nano_v0_8.onnx
    voices.npz
    config.json
```

从 [KittenML/kitten-tts-nano-0.8](https://huggingface.co/KittenML/kitten-tts-nano-0.8/tree/main)
下载 `kitten_tts_nano_v0_8.onnx`、`voices.npz` 和 `config.json`。也可换用同系列其他
轻量 ONNX 模型，并通过启动参数指定文件路径。

## 启动

```powershell
go mod tidy
go run .
```

默认地址为 <http://127.0.0.1:8080>。

常用参数：

```powershell
go run . `
  -addr 127.0.0.1:8080 `
  -onnx-lib C:\path\to\onnxruntime.dll `
  -espeak C:\path\to\espeak-ng.exe `
  -kokoro-model models\kokoro\kokoro-v0_19.onnx `
  -kokoro-voices models\kokoro\voices-v1.0.bin `
  -kitten-model models\kitten\kitten_tts_nano_v0_8.onnx `
  -kitten-voices models\kitten\voices.npz `
  -kitten-config models\kitten\config.json
```

完整参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8080` | Web 监听地址 |
| `-onnx-lib` | 自动查找 | ONNX Runtime 动态库 |
| `-espeak` | 自动查找 | 音素化可执行文件 |
| `-kokoro-model` | `models/kokoro/kokoro-v0_19.onnx` | Kokoro 模型 |
| `-kokoro-voices` | `models/kokoro/voices-v1.0.bin` | Kokoro 音色文件或目录 |
| `-kitten-model` | `models/kitten/kitten_tts_nano_v0_8.onnx` | Kitten 模型 |
| `-kitten-voices` | `models/kitten/voices.npz` | Kitten 音色文件或目录 |
| `-kitten-config` | `models/kitten/config.json` | Kitten 音色别名与语速先验 |
| `-edge-voice` | `zh-CN-XiaoxiaoNeural` | 初始 Edge 音色 |
| `-edge-proxy` | 空 | Edge TTS 使用的 HTTP 代理 |
| `-max-chars` | `5000` | 单次输入字符上限 |

也支持环境变量：`TTS_ADDR`、`ONNXRUNTIME_LIB`、`ESPEAK_NG`、`KOKORO_MODEL`、
`KOKORO_VOICES`、`KITTEN_MODEL`、`KITTEN_VOICES`、`KITTEN_CONFIG`、`EDGE_VOICE`
和 `EDGE_PROXY`。

Windows 便携版 eSpeak 可放在 `runtime/espeak-ng/espeak-ng.exe`。应用会自动找到同目录下的
`espeak-ng-data` 并传给子进程，无需手工设置环境变量。

## HTTP 接口

`POST /api/synthesize`，使用 `application/x-www-form-urlencoded`：

```text
engine=edge
voice=zh-CN-XiaoxiaoNeural
text=你好，世界
speed=1.0
```

成功时直接返回 `audio/mpeg` 或 `audio/wav`，失败时返回 JSON：

```json
{"error":"具体错误信息"}
```

其他接口：

- `GET /api/voices?engine=edge|kokoro|kitten`
- `GET /healthz`

## 测试

默认测试不访问网络，也不需要加载几百 MB 的模型：

```powershell
go test ./...
go test -race ./...
go vet ./...
```

使用本机已下载的 Kokoro、Kitten 和 ONNX Runtime 运行真实推理测试：

```powershell
$env:TTS_INTEGRATION = "1"
go test ./internal/tts -run TestLocalEngineIntegration -v
```

调用真实 Edge TTS 服务：

```powershell
$env:TTS_NETWORK_INTEGRATION = "1"
go test ./internal/tts -run TestEdgeLiveIntegration -v
```

两类集成测试在未设置对应环境变量时会自动跳过，适合普通 CI 和离线开发。

## 实现说明

- Web 路由、Controller、Filter、静态资源和安全响应头由 `go-web-frame` 管理。
- 为保留 `-addr` 的主机绑定能力，使用框架生成的 `http.Handler` 配合标准
  `net/http.Server` 启动；路由与请求处理仍完整经过框架。
- Edge TTS 直接使用 WebSocket 协议，包含当前 `Sec-MS-GEC` v2 令牌生成，不依赖 Python。
- 长文本按句拆分；Kokoro 还会在音素 token 超过模型限制时继续递归拆分。
- Kokoro 输出 24 kHz float32 PCM，服务端转为单声道 16-bit WAV。
- Edge 输出 `audio-24khz-48kbitrate-mono-mp3`；长文本的 MP3 分片在服务端顺序拼接。
- 本地音色 NPZ/NPY 使用 Go 标准库解析，不依赖 NumPy。

Edge Read Aloud 是非正式接口，服务端行为可能变化，并受微软相关条款约束。Kokoro、
Kitten 的模型文件许可与代码许可相互独立，请在使用或分发前分别确认。
