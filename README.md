# WattScope

跨平台 USB 电流表上位机，支持 **WITRN K2** 与 **POWER-Z KM003C**，提供原生 GUI、终端界面（TUI）和命令行工具，覆盖 Linux、macOS、Windows 与 Android。

## 功能

- **实时监测**：功率、电压、电流、温度与 D± 曲线，支持时间轴缩放、暂停与会话统计
- **USB PD 分析**：被动抓包、报文解码（含扩展消息重组）、SPR AVS / EPR 档位、PPS 输出状态、协商时间线与常见异常提示
- **录制与离线查看**：CSV 测量录制；PD 抓包保存、导出与离线回放
- **多语言**：简体中文、繁體中文、English、日本語

## 快速开始

需要 Go 1.27.3 或更新版本：

```sh
go build -o build/wattscope ./cmd/wattscope
./build/wattscope              # 打开 GUI
./build/wattscope gui --demo   # GUI + 演示数据
./build/wattscope tui --demo   # TUI + 演示数据
```

## 命令行

```sh
wattscope devices                        # 列出已连接设备
wattscope read --json                    # 实时输出 JSON 读数
wattscope record -o charge.csv           # 录制测量数据到 CSV
wattscope pd capture -o trace.pd.jsonl   # 抓取 PD 报文
wattscope pd view trace.pd.jsonl         # 离线查看抓包
wattscope pd analyze trace.pd.jsonl      # 分析 PD 协商过程
```

全局选项：`--demo`（模拟设备）、`--lang`（界面语言）、`--recordings-dir`（录制目录）。

## 构建与打包

```sh
go tool mygo build -platform darwin/arm64    # macOS .app
```

**Android GUI** 使用仓库根目录的 `wattscope.p12` 签署 APK 和 AAB。构建时输入该密钥的密码。

```sh
(
  printf 'Android 签名密码: '
  IFS= read -rs MYGO_ANDROID_KEYSTORE_PASSWORD || exit 1
  printf '\n'
  export MYGO_ANDROID_KEYSTORE_PASSWORD
  go tool mygo build -platform android/arm64
)
```

**Windows**（amd64 示例，CLI/TUI 与 GUI 分开编译）：

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -tags no_gui -ldflags='-s -w -H=windows' -o build/wattscope-windows-amd64.exe ./cmd/wattscope
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -tags no_cli -ldflags='-s -w -H=windowsgui' -o build/wattscope-gui-windows-amd64.exe ./cmd/wattscope
```

**Android CLI**（arm64，交叉编译后通过 ADB 运行）：

```sh
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -tags no_gui -ldflags='-s -w' -o build/android/wattscope-cli-android-arm64 ./cmd/wattscope
adb push build/android/wattscope-cli-android-arm64 /data/local/tmp/wattscope
adb shell chmod 755 /data/local/tmp/wattscope
adb shell /data/local/tmp/wattscope devices
adb shell "su -c '/data/local/tmp/wattscope read --json'"
```

采集需要设备节点权限（K2 用 `/dev/hidraw*`，KM003C 用 `/dev/bus/usb/*/*`），权限不足时用已授权的 `su`，同时受 SELinux 策略影响。演示与离线分析无需权限。

## 设备与平台提示

- **WITRN K2**：经侧面接口连接；抓取 PD 报文前，先在表端开启「**PD 连机**」。
- **POWER-Z KM003C**：经侧面数据口连接。Windows 直连需为设备接口绑定 WinUSB。
- **Linux**：提示缺少权限时，在「设置 → 权限」点击「授权」并输入管理员密码。WattScope 会安装 udev 规则（`/etc/udev/rules.d/70-wattscope.rules`）并刷新已连接的设备，之后在本机桌面登录的用户无需 root 即可使用 GUI、CLI 与 TUI。需要 `pkexec` 与桌面 Polkit 认证代理。
  若内核 `powerz` 驱动占用 KM003C 导致无法抓取 PD，可先临时解绑该 USB 接口。
- **Android GUI**：启动后自动识别并连接。使用 root 模式的 Shizuku 或 Sui 时，可在「设置 → 权限」授权 WattScope，之后直接打开 USB 接口，跳过系统确认框；否则在设备页手动连接并通过系统弹窗授权。

## 文档

[架构与扩展](docs/architecture.md)

## 许可证

[GNU GPL v3.0](LICENSE)

## 致谢

感谢 [MyGo](https://github.com/egoist/mygo) 提供跨平台 GUI 框架。
