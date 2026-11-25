# Changelog

XTerm is a secondary development based on the open source project
[FinallyShell](https://github.com/TooonyChen/finallyshell). Its own versions
start at 0.0.1; the upstream project's history stays in
[its repository](https://github.com/TooonyChen/finallyshell/releases).

Each version's notes, newest first, in English and Simplified Chinese. A
release's notes on GitHub and in the app's update window are its section
here; see [发布](docs/zh-CN/development.md#发布).

## 0.1.1

### English

- The server monitor's network and latency are now two cards: smooth curves with a faded fill, the download and upload speeds, the link's speed, the traffic of the day, and each graph's peak and span.
- The CPU, memory, disk and swap bars and rings use the new palette: blue, purple, green and grey.
- The top five processes are gone from the monitor: the Processes tab has them all.
- macOS is supported by the monitor: the system panel, the process list and the process details read ps, vm_stat, netstat and df.
- A server whose ps cannot list every process, as OpenWrt's BusyBox ps may not, lists them from /proc: the Processes tab is no longer empty there.
- Double-clicking a process, or its menu's Details, opens its details in a dialog: the process, its command line, its paths and the buttons to copy, lower its priority or signal it.
- Each signal in the menu now says what it does: gracefully quit, force kill, reload the configuration, interrupt, pause or resume.
- Terminal text highlighting: rules that color log levels, IP addresses, times, permissions, sizes, links, keywords and numbers, with the presets' switches and your own rules in Options.

### 简体中文

- 服务器监控的网络和延迟改成两张卡片：平滑曲线和淡淡的填充、上下行速率、网卡速率、今日流量，以及每个图的峰值和时长。
- CPU、内存、磁盘和交换的进度条与圆环换成新配色：蓝、紫、绿、灰。
- 监控里的进程 TOP 5 去掉了：进程页里有全部进程。
- 监控支持 macOS：系统面板、进程列表和进程详情分别读取 ps、vm_stat、netstat 和 df。
- 服务器上的 ps 列不出所有进程时（OpenWrt 的 BusyBox ps 就可能这样），改用 /proc 列出，进程页不再是空的。
- 双击进程或右键“打开详情”，在弹窗里显示进程详情：进程、命令行、路径，以及复制、降低优先级、发送信号等按钮。
- 菜单里的每个信号都注明了作用：优雅退出、强制结束、重载配置、中断、暂停、恢复。
- 终端文本高亮：按规则给日志级别、IP 地址、时间、权限、大小、链接、关键词和数字着色，预设可开关，也可以自己加规则。

## 0.0.1

### English

- The first release of XTerm, a secondary development of FinallyShell: the
  server monitor, the sessions and the saved connections in the left panel,
  the terminal with split panes, remote files over SFTP, quick commands, the
  command bar, tunnels, sync and the connection importers.
- Options › About shows the version and checks this repository's Releases
  for updates.
- The app is named XTerm, with its own icon; settings and passwords saved by
  the earlier name are carried over.

### 简体中文

- XTerm 的第一个版本，基于 FinallyShell 二次开发：服务器监控、左侧面板里的会话和
  保存的连接、支持分屏的终端、SFTP 远程文件、快捷命令、命令输入框、隧道、同步和
  连接导入。
- “选项 › 关于”里显示版本，并检查本仓库的 Releases 进行更新。
- 应用名改为 XTerm，使用新的图标；旧名称下保存的设置和密码会自动沿用。
