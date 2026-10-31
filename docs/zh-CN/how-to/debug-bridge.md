# 排查工作台连接失败

可执行程序、浏览器扩展和工作台需要建立完整连接。能打开工作台网页，只能说明页面可访问，不能证明 Native Messaging 或浏览器认证转发正常。

## 1. 确认可执行文件和版本

确认实际启动程序的位置与版本。安装后移动程序，会让 Native Messaging 注册仍指向旧路径。

移动后重新运行目标程序的 `install` 命令。让扩展与程序版本一致，重新加载扩展，关闭旧工作台标签，再从扩展打开工作台。

## 2. 检查本地服务

默认地址属于运行 Bridge 的机器。Windows PowerShell：

```powershell
Invoke-RestMethod http://127.0.0.1:32145/v1/health
```

Linux 或 macOS：

```bash
curl --fail http://127.0.0.1:32145/v1/health
```

连接被拒绝时，先打开扩展以触发 Native Messaging，再检查注册、文件路径、执行权限和启动错误。接口能响应也可能是旧进程，需核对其版本。

## 3. 检查扩展

在 `edge://extensions` 或 `chrome://extensions` 确认 BlogCTL 已启用，解压目录仍然存在。扩展错误和 service worker 控制台可用于定位本地主机启动失败。替换文件后需要重新加载扩展。

健康接口正常、工作台操作失败时，检查扩展转发和旧标签页。从当前扩展重新打开工作台，确认连接状态后再提交任务。

## 4. 检查工作区和账号

工作台已连接，仍可能遇到目录不可访问或平台登录过期。先核对设置及[工作区指南](configure-workspace.md)，再查看失败任务的平台错误。Windows 程序必须使用 Windows 可访问的路径；WSL 路径不会自动转换。

可以主动执行只读检测或列表操作检查登录状态。它不是每次创建前的必做步骤。平台要求人机验证时，在真实浏览器中完成后再重试读取。

## 5. 保留排查信息

通过日志和任务记录定位失败层级，记录版本、操作、时间与脱敏错误。不要导出 Cookie、Token、Native Messaging 认证数据或未脱敏的配置文件。

升级时保留 `Data/`。删除它会丢失配置与发布历史，不应作为连接修复步骤。

另见 [Bridge 模型](../concepts/bridge-and-state.md)和[配置参考](../reference/configuration.md)。
