# 安装 BlogCTL 并连接浏览器

BlogCTL 包含本地 Go 程序、Edge/Chrome 扩展和 Web Console。依赖浏览器账号的操作需要扩展；网站构建还需要 Inkerio 源码和 Node/npm。

## 1. 下载匹配文件

从 [Releases](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io/releases/latest) 下载对应系统/CPU 的程序，以及**同一个 BlogCTL Release** 的扩展 ZIP。六种二进制覆盖 Windows、Linux、macOS 的 amd64/arm64。将扩展解压到固定位置。

Release 程序无需安装 Go。Windows 建议先把程序放到长期使用的位置，例如 `C:\software\Coding\blogctl\`。相邻的 `Data/` 保存配置和任务，升级时保留。

从源码构建时，在引擎根目录运行：

```bash
go run ./tools/blogctl/cmd/maint build
.build/out/blogctl help
```

上述命令使用 Bash。Windows 需要带 `.exe` 的程序时，可用 `go build -o blogctl-windows-amd64.exe ./tools/blogctl/cmd` 构建；匹配扩展使用同一源码目录的 `tools/blogctl/extension/`。

## 2. 注册 Native Messaging

Windows PowerShell：

```powershell
& 'C:\software\Coding\blogctl\blogctl-windows-amd64.exe' install
```

Linux 示例：

```bash
chmod +x ./blogctl-linux-amd64
./blogctl-linux-amd64 install
```

macOS 使用对应的 `blogctl-darwin-*` 文件。`install` 把当前程序注册为浏览器 Native Messaging Host，不会替你加载扩展。移动程序后重新注册。`uninstall` 解除 Host 注册，保留用户数据。

## 3. 加载并打开扩展

1. 在准备使用的浏览器配置中打开 `edge://extensions` 或 `chrome://extensions`。
2. 开启开发者模式，点击「加载已解压的扩展程序」，选择包含 `manifest.json` 的目录。
3. 打开 BlogCTL，扩展会按需启动、连接 Bridge。
4. 确认已连接，再点击「打开工作台」。

工作台地址为 `http://127.0.0.1:32145/console/`。保持扩展启用；直接打开 URL 不会自动建立浏览器转发通道。

## 4. 设置工作区

在「设置」填写 **Engine Root**（Inkerio 引擎）和 **Content Root**（内容仓库）。路径必须能被程序所在系统访问：Windows 程序使用 Windows 路径，WSL Linux 程序使用 Linux 路径。保存后重新打开设置，确认持久化成功。

在引擎目录运行 `blogctl doctor` 检查 Node/npm/Git。与 `help`、`install` 不同，当前 `doctor` 需要在引擎工作目录运行。

## 5. 验证与升级

PowerShell：

```powershell
Invoke-RestMethod http://127.0.0.1:32145/v1/health
```

Bash：

```bash
curl http://127.0.0.1:32145/v1/health
```

健康响应说明 HTTP 服务正常，还需检查扩展连接指示器。这一步不需要发布平台账号。

升级时保留 `Data/`，一起替换程序和扩展，再从浏览器扩展页重新加载、打开工作台。程序位置变化时重新注册。排查版本不一致时，先确认没有旧 Bridge 进程仍在运行。

下一步查看[创建首个草稿](tutorial/first-publish.md)、[新机器教程](tutorial/first-run-on-new-machine.md)或[连接排查](how-to/debug-bridge.md)。

## 可选：使用 ChatGPT MCP

安装版本匹配的 BlogCTL 程序和 Edge/Chrome 扩展后，可通过独立的 Secure MCP Tunnel 启动 `blogctl -mcp-stdio`。MCP 调用已有 Bridge，并由浏览器扩展同步 Cookie。不要在 WSL 启动独立 Bridge 代替 Windows 的浏览器登录态。参见 [MCP 接入](../MCP.md)。

需要图片上传时，配置 `engine_root`、Node.js 和 Sharp，以及 TOML 顶层 `image_upload_roots` 白名单；先用 `dry_run=true` 验证清理结果。参见 [MCP 图片上传](../MCP.md#private-image-upload-to-r2)。
