# Changelog

XTerm is a secondary development based on the open source project
[FinallyShell](https://github.com/TooonyChen/finallyshell). Its own versions
start at 0.0.1; the upstream project's history stays in
[its repository](https://github.com/TooonyChen/finallyshell/releases).

Each version's notes, newest first, in English and Simplified Chinese. A
release's notes on GitHub and in the app's update window are its section
here; see [发布](docs/zh-CN/development.md#发布).

## 0.3.3

### English

- Connect to an object storage as a host: Aliyun OSS, Tencent Cloud COS, Huawei Cloud OBS, Qiniu Kodo, AWS S3, Google Cloud Storage, Azure Blob, MinIO, RustFS, and a directory of this machine.
- An object storage's tab is its files, with the file panel's table: open a folder, download with a double click, upload files or drop them in, make a folder, delete; there is no terminal and no bottom panel.
- The storage's System page probes the bucket every ten seconds — online, no access, offline or timed out, and how long it took — and draws the success rate, the traffic, the calls a second of Put, Get, List and Delete, their P50/P90/P99, and the answers by status code.
- Uploads and downloads list their files under the panel: a bar, the bytes, the speed and the time left for each, with a cancel button and a way to clear the finished ones; the traffic counts bytes as they flow, not when a transfer ends.
- A cloud's region is a list: choosing one fills in that region's endpoint, the regions of AWS China included; a region the list does not have is typed.
- Redis servers get a System page of their own: the role, both versions, the clients connected and the uptime, the memory, clients and hit rate as bars and rings, the commands a second and the traffic as curves, and a bar for each database's keys.
- INFO's sections are a button that opens a dialog: the sections at its top, and the fields of the chosen one as a table of the field, its value and what it means — the fields of Redis 6 to 8 are documented, and a field of a newer server says that it is not documented yet.
- The connection tree, the recent connections, the import preview and the object panel show each connection's own logo — Linux, Redis, the cloud of the bucket — instead of the same server icon.

### 简体中文

- 可以把对象存储当作一台主机来连接：阿里云 OSS、腾讯云 COS、华为云 OBS、七牛云 Kodo、AWS S3、Google Cloud Storage、Azure Blob、MinIO、RustFS，以及本机的一个目录。
- 对象存储的标签页就是它的文件，和文件面板同样的表格：进入文件夹、双击下载、上传或把文件拖进来、新建文件夹、删除；没有终端，也没有底部面板。
- 对象存储的系统页每十秒探测一次桶，显示在线、无权限、离线或超时和这次探测的耗时，并画出成功率、流量、Put / Get / List / Delete 每秒的请求量与 P50/P90/P99，以及按状态码统计的应答。
- 上传和下载在面板下方列出每个文件：进度条、字节数、速度和剩余时间，可以取消，全部结束后可以清除；流量按传输中的字节实时增长，不是传完才记。
- 云厂商的区域改成下拉：选中就把服务地址填成该区域的端点，包括 AWS 中国区；列表里没有的区域选“自定义…”后输入。
- Redis 服务器有自己的系统页：角色、两个版本、已连接客户端数和运行天数，内存、客户端数和命中率画成用量条与环，命令量和流量画成曲线，每个数据库的键数一根条。
- INFO 的分区变成一个按钮，点开是对话框：上面是各分区，下面是所选分区的字段、数值和含义三列表格——Redis 6 到 8 的字段都有说明，比 8 更新的服务器上的字段显示“暂无说明”。
- 连接树、最近连接、导入预览和对象存储面板显示各自的标志——Linux、Redis、桶所在的云——不再是同一个服务器图标。

## 0.2.2

### English

- The AI tab shows each answer as the page it is written as: headings, lists, quotes and tables, code blocks with their language and a copy button, and bold, italic, inline code and links within a paragraph.
- An answer still streaming shows what it has: an open code block runs to its end, and markers that have not closed keep their text.
- A long answer scrolls above the question box: the box, the send button and the new chat button stay inside the panel instead of being pushed out of it.

### 简体中文

- AI 标签页把每个回答按它写的页面显示：标题、列表、引用和表格，代码块带语言和复制按钮，段落里的粗体、斜体、行内代码和链接。
- 回答还在生成时也照样显示：没闭合的代码块一直显示到末尾，没闭合的标记保留原文。
- 回答再长也在输入框上方滚动：输入框、发送按钮和新建对话按钮留在面板里，不再被顶到面板外面。

## 0.2.1

### English

- An AI assistant answers questions about the server. Options › AI chooses an OpenAI-compatible service (DeepSeek, OpenAI, Kimi, GLM, SiliconFlow and Ollama are set up), the model and its API key, and what the assistant may do.
- Each session has its own AI tab in the bottom panel, after Commands: every tab and host keeps its own conversation, so different servers never mix.
- Answers stream in with the model's reasoning, and can be copied, regenerated, or inserted into the terminal to run.
- With commands allowed, the assistant runs read-only commands on the session's host, and reads the monitor's snapshot and the file panel's folder; by default each command waits for your confirmation, and it runs on a channel of its own without touching the terminal.
- The question box sends with Enter, breaks the line with Ctrl+Enter, and keeps its send button at the end of the box.
- A session's menu records its transcript, or Options › Recording records every session as it connects; the recordings are listed there to open or delete.
- A session's menu also diagnoses the connection: the round trip, the ports this computer can reach, and what the server's own DNS and route say.
- The connection editor gives jump hosts a page of their own, showing the chain a connection goes through.
- The manager's menu exports the saved connections as JSON, folders and hosts, without passwords.
- A process's details show the chain that started it, from init down to the process, with each step's command.
- A terminal that lost its connection keeps its screen and history instead of going blank, ready to reconnect.
- The files' columns sort by a click on their headers.

### 简体中文

- 新增 AI 助手，回答服务器相关的问题。“选项 › AI”里选择 OpenAI 兼容的服务（已内置 DeepSeek、OpenAI、Kimi、GLM、SiliconFlow 和 Ollama）、模型和 API Key，以及助手能做什么。
- 每个会话在底部面板“命令”之后都有自己的 AI 标签页：每个标签、每台服务器各有一份对话，互不混杂。
- 回答边生成边显示，连同模型的思考过程；可以复制、重新生成，或插入终端执行。
- 允许执行命令时，助手会在会话的主机上运行只读命令，读取监控快照和文件面板当前目录；默认每条命令都要确认，并且在独立通道执行，不打扰终端。
- 输入框按 Enter 发送，按 Ctrl+Enter 换行，发送按钮就在输入框末尾。
- 会话右键菜单可以录制终端记录，“选项 › 录制”可以让每个会话连接时自动开始；录制文件在选项里打开或删除。
- 会话右键菜单还能诊断连接：往返延迟、本机可达的端口，以及服务器自己的 DNS 和路由结果。
- 连接编辑器里跳板机有了独立页面，显示连接经过的跳转链。
- 连接管理器的菜单可以把保存的连接导出为 JSON，包含文件夹和主机，不含密码。
- 进程详情显示它的来源链：从 init 一路到该进程，每一步的命令。
- 断线的终端会保留最后的画面和历史，而不是变成空白，随时可以重连。
- 文件列表的列可以点表头排序。

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
