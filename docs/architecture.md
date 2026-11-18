# 架构

## 分层

```text
cmd/wattscope → cli / tui / app(GUI) → session → backend → recording
                                          └→ driver → device ← transport
                       pdview, protocol/pd, locales（共用）
```

| 包 | 职责 |
| --- | --- |
| `session` | 前端共用入口：注册驱动、创建 `device.OpenFunc` 并交给 `backend.New` |
| `backend` | 采集会话、统计、CSV 录制、PD 抓包 |
| `driver/<厂商>/<型号>` | 设备私有协议；K2 见 [protocol.md](../internal/driver/witrn/k2/protocol.md) |
| `transport` | 枚举、打开、读取、关闭（hid、usbhost），不识别设备命令 |
| `protocol/pd` | PD 报文解码与协商分析，不依赖驱动 |
| `pdview` | PD 报文说明与筛选，CLI / TUI / GUI 共用 |
| `recording` | CSV 与抓包文件读写、录制路径解析 |
| `locales` | 多语言目录（`en.json` 为源，缺失回退英文） |
| `kit` | GUI 组件与样式层（见下文） |

前端只依赖通用设备接口与能力字段，不感知具体型号。

## 增加新设备

1. 在 `internal/driver/<厂商>/<型号>/` 实现 `device.Driver`（`ID`、`Name`、`Discover`、`Open`）。
2. `Open` 返回的 `device.Connection` 实现 `Read(ctx)` 与 `Close()`，输出通用 `device.Sample`，读取须响应取消与超时。
3. 在 `session.Drivers` 注册，采集、重连、CSV、TUI 即可直接使用。

约定：

- `device.Info.ID` 是接口的稳定身份，不能只用序列号；`Open` 须重新核对实际设备。
- `Voltage`、`Current` 必需且为非负幅值，方向由 `CurrentDirection` / `SignedPower` 表示；温度、D±、累计 Ah/Wh 等可选值用指针，不支持时为 nil。驱动应拒绝非法或非有限值。
- `ErrIgnoredReport` / `ErrInvalidReport` 由后端计数并继续读取，不延长连接超时；其他错误结束连接，按配置重连。
- 普通接收不得主动发送固件命令。
- 支持 PD 的驱动额外实现 `EventConnection.ReadEvent(ctx)` 并声明 `pd_capture`；同一连接只有一个读取协程。
- Linux USB 驱动通过 `LinuxUSBMatches()` 提供驱动已有的 VID/PID 和节点类型。GUI 通过 `pkexec` 调用 WattScope 的授权子命令生成规则并刷新设备。
- 新传输（串口、BLE 等）放在 `internal/transport`，不改通用接口。

## 后端会话

`backend.Service` 对应一次会话，`Run(ctx)` 只运行一次；断线重连属于同一会话但开启新的连续采集段。

- **快照与通知**：`Snapshot` / `SnapshotWindow` 返回独立副本；`Subscribe` 的通知是合并的，收到后应重新取快照。
- **录制与抓包**：`StartRecording` / `StartCapture` 并发安全，结束时统一刷新关闭。CSV 增量写入；PD 报文在测量限频前直接写入 NDJSON。
- **统计**：梯形积分；重连、计数回退、组号变化、超时断档不连成连续时间。
- **PD**：坏 CRC / 坏长度单独统计并保留原始数据，不延长超时，也不当作电压读数。`protocol/pd` 的 `Analyzer` 为每包附加与语言无关的分析（协商阶段、请求参数、提示代码）；时间仅表示主机接收间隔。
- **错误**：`Snapshot.ErrorKind` 保持稳定，界面负责翻译；原始错误保留在设备详情。
- **离线**：`pd view` / `pd analyze` 经 `recording.ReadCapture` 流式读取并按原始字节重新分析，忽略文件内旧分析结果。

## 前端

- **TUI**：宽屏左侧导航、窄屏顶部页签；含测量页与 PD 页（选包、暂停、筛选、协商时间线、导出）。布局与鼠标命中共用尺寸规则，中文按显示宽度计算。
- **GUI**（MyGo）：与 CLI 共用 `cmd/wattscope`，页面与导航见 `internal/app/dashboard.go`。采集在工作协程运行，通知触发 `Invalidate()`；切换数据源会取消旧会话，待其退出后再打开设备。Android 通过 `transport/usbhost` 请求 USB 授权。
- **语言**：通过 `locales.In(locales.Resolve(tag))` 显式选择，不修改 MyGo 全局回调；新增语言只需添加 BCP 47 命名的 JSON 并重新生成访问器（`mygo i18n check` 校验）。

## GUI 组件（`internal/kit`）

`kit` 是 GUI 唯一的组件与样式来源。`internal/app` 只负责组合组件、填充数据，主题、尺寸、动效与适配统一在 `kit` 中维护。

| 文件 | 内容 |
| --- | --- |
| `theme.go` | 调色板、圆角、控件尺寸、断点 |
| `components.go` | 卡片、表单行、徽标、按钮、分段控件、空状态、Toast |
| `navigation.go` | 侧栏、顶栏、页签、底部导航 |
| `overlay.go` | 菜单、弹出层、对话框、底部抽屉 |
| `glass.go` | 半透明表面及回退样式 |
| `motion.go` / `theme_reveal.go` | 动效预设、主题切换动画 |
| `icons.go` | 图标 |

约束：

1. **复用即下沉**：同一控件或布局在 `app` 中出现两次以上，就抽成 `kit` 组件。
2. **页面不写样式**：颜色取 `Palette`，圆角用 `Radius*`，高度用 `ControlHeight` / `TabHeight`；不硬编码色值、间距或动画参数。
3. **适配内聚**：触控、窄屏、深浅色、强调色由组件内部处理，调用方只传 `touch` 等参数。
4. **交互统一**：弹层与 Toast 只用 `overlay.go` / `ToastViewport`，动效只用 `motion.go` 预设。
5. **保持纯 UI**：`kit` 不依赖业务包、不含文案；文字由调用方经 `locales` 传入。

## 平台与验证状态

- **Linux**：GUI 通过 Polkit 为已注册的 USB 驱动生成 `uaccess` 规则。hidraw 遥测以只读方式打开，真实 K2 遥测已验证。PD 抓包仅通过样本、演示流与 TUI 交互验证，未验证真实 PD 输入。
- **Windows**：系统 HID + 可取消 overlapped I/O，仅交叉编译通过。
- **macOS**：基于 `purego` 调用 IOKit，`CGO_ENABLED=0` 构建，不依赖 C 工具链；CI 在 Linux 交叉编译，真实设备枚举、授权与 PD 上报待实机验证。

所有平台只能保存传输层已交给应用的报告，无法保证设备侧或系统 HID 队列无漏包。
