# Bridge、浏览器身份与运行状态

Go Bridge 负责本地配置、任务和发布适配；浏览器扩展负责已登录平台的会话身份。Web Console 和扩展侧边栏使用相同的 UI。

```text
Edge / Chrome Extension
        ↕ 受限消息通道
Web Console（共享 UI） → 本地 Go Bridge
                              ├── 本地任务与配置
                              └── 各平台 Adapter
```

Web Console 无权直接获取浏览器 Cookie 或高权限 Bridge Token。关闭扩展后，页面仍可能显示，但需要认证的操作会连接失败。

## 本地数据

Windows 默认把数据放在可执行文件旁的 `Data/`；Linux/macOS 使用系统用户配置目录。也可以通过绝对路径环境变量 `BLOGCTL_DATA_DIR` 指定新目录。

- `blogctl.toml`：配置
- `jobs.json`：任务状态
- `publications.json`：持久发布记录
- `distribution/`、日志：运行时结果

数据目录可能包含凭据，不应加入版本控制。实际路径由 [`bridge/config.go`](../../../bridge/config.go) 决定；具体键值见[配置参考](../reference/configuration.md)。
