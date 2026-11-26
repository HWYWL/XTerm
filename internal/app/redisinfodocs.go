package app

import (
	"strings"

	"github.com/egoist/mygo/i18n"
)

// redisDoc is what an INFO field means, in the two languages of the app.
type redisDoc struct{ en, zh string }

// redisFieldDocs are what the fields of INFO mean, as redis.io documents
// them: those of Redis 6 to 8, section by section, and the shapes of the
// ones whose name is the value's, as db0 and errorstat_ERR.
var redisFieldDocs = map[string]redisDoc{
	// Server.
	"redis_version":      {"Version of the Redis server", "Redis 服务器版本"},
	"redis_git_sha1":     {"Git SHA of the build, 0 for a release", "构建所用的 Git 版本，发布版为 0"},
	"redis_git_dirty":    {"Whether the build had local changes", "构建时是否有未提交的改动"},
	"redis_build_id":     {"Identifier of the build", "构建标识"},
	"redis_mode":         {"standalone, cluster or sentinel", "运行模式：单机、集群或哨兵"},
	"os":                 {"Operating system and kernel of the server", "服务器的操作系统与内核版本"},
	"arch_bits":          {"Bitness of the binaries: 32 or 64", "可执行文件的位数：32 或 64"},
	"monotonic_clock":    {"Clock the server times with, as POSIX", "服务器计时所用的单调时钟类型，如 POSIX"},
	"multiplexing_api":   {"Event loop the server uses", "服务器使用的事件循环"},
	"atomicvar_api":      {"Atomic implementation, as C11", "原子操作实现，如 C11"},
	"gcc_version":        {"Compiler that built the server", "编译服务器所用的编译器版本"},
	"process_id":         {"PID of the server", "服务器进程号"},
	"process_supervised": {"How it is supervised: no, upstart or systemd", "进程管理方式：无、upstart 或 systemd"},
	"run_id":             {"Random identifier of this run", "本次运行的随机标识"},
	"tcp_port":           {"Port it listens on", "监听的 TCP 端口"},
	"server_time_usec":   {"Its clock, in microseconds", "服务器时钟（微秒）"},
	"uptime_in_seconds":  {"Seconds since it started", "已运行的秒数"},
	"uptime_in_days":     {"Days since it started", "已运行的天数"},
	"hz":                 {"Background tasks per second, as the server set it", "每秒后台任务次数（实际）"},
	"configured_hz":      {"Background tasks per second, as configured", "每秒后台任务次数（配置）"},
	"lru_clock":          {"Clock for the LRU of the keys, in seconds", "键的 LRU 时钟（秒）"},
	"executable":         {"Path of the server's binary", "服务器可执行文件路径"},
	"config_file":        {"Configuration file it was started with", "启动所用的配置文件"},
	"io_threads_active":  {"Whether IO threads are in use", "是否启用 IO 线程"},
	"listener0":          {"A listening socket: its name, address and port", "监听端口：名称、绑定地址与端口"},

	// Clients.
	"connected_clients":               {"Clients connected now, this one included", "当前连接数（含本连接）"},
	"cluster_connections":             {"Connections of the cluster bus", "集群总线连接数"},
	"maxclients":                      {"Clients it takes at once", "允许的最大连接数"},
	"client_recent_max_input_buffer":  {"Largest input buffer of a client", "单个客户端最大的输入缓冲"},
	"client_recent_max_output_buffer": {"Largest output buffer of a client", "单个客户端最大的输出缓冲"},
	"blocked_clients":                 {"Clients waiting on BLPOP and the like", "阻塞中的客户端，如等待 BLPOP"},
	"tracking_clients":                {"Clients tracking keys for client side caching", "开启键跟踪（客户端缓存）的客户端数"},
	"pubsub_clients":                  {"Clients subscribed to channels", "处于订阅模式的客户端数"},
	"watching_clients":                {"Clients watching keys with WATCH", "用 WATCH 监视键的客户端数"},
	"clients_in_timeout_table":        {"Clients in the timeout table", "超时表中的客户端数"},
	"total_watched_keys":              {"Keys watched by the clients", "被客户端监视的键数"},
	"total_blocking_keys":             {"Keys that clients wait on", "有客户端在等待的键数"},
	"total_blocking_keys_on_nokey":    {"Waiting clients of keys that do not exist yet", "等待尚不存在的键的客户端数"},

	// Memory.
	"used_memory":                   {"Memory the server allocated, in bytes", "服务器分配的内存（字节）"},
	"used_memory_human":             {"The same, in K/M/G", "同上，以 K/M/G 显示"},
	"used_memory_rss":               {"Memory the OS gave it, in bytes", "操作系统实际给它的内存（字节）"},
	"used_memory_rss_human":         {"The same, in K/M/G", "同上，以 K/M/G 显示"},
	"used_memory_peak":              {"Largest it allocated since it started", "启动以来分配过的峰值（字节）"},
	"used_memory_peak_human":        {"The same, in K/M/G", "同上，以 K/M/G 显示"},
	"used_memory_peak_perc":         {"Now of that peak, in percent", "当前占峰值的百分比"},
	"used_memory_overhead":          {"Memory for its own structures", "内部管理结构占用的内存"},
	"used_memory_startup":           {"Memory at startup, with no data", "启动时（无数据）占用的内存"},
	"used_memory_dataset":           {"Memory the data takes", "数据本身占用的内存"},
	"used_memory_dataset_perc":      {"That, of the memory after startup", "数据占启动后内存的百分比"},
	"allocator_allocated":           {"Bytes the allocator gave out", "分配器已分配出去的字节数"},
	"allocator_active":              {"Pages the allocator holds", "分配器占用的页大小"},
	"allocator_resident":            {"Pages the OS gave the allocator", "操作系统给分配器的常驻页大小"},
	"allocator_muzzy":               {"Memory the allocator gave back, reusable", "分配器已归还但可复用的内存"},
	"total_system_memory":           {"Memory of the machine, in bytes", "机器总内存（字节）"},
	"total_system_memory_human":     {"The same, in K/M/G", "同上，以 K/M/G 显示"},
	"used_memory_lua":               {"Memory of the Lua engine", "Lua 引擎占用的内存"},
	"used_memory_vm_eval":           {"Memory of the scripting VM", "脚本虚拟机占用的内存"},
	"used_memory_lua_human":         {"The Lua engine's, in K/M/G", "Lua 引擎占用的内存，以 K/M/G 显示"},
	"used_memory_scripts_eval":      {"Memory of the cached scripts", "缓存脚本占用的内存"},
	"number_of_cached_scripts":      {"Scripts cached (EVALSHA)", "缓存的脚本数（EVALSHA）"},
	"number_of_functions":           {"Functions the libraries hold", "库中定义的函数数"},
	"number_of_libraries":           {"Function libraries loaded", "加载的函数库数"},
	"used_memory_vm_functions":      {"Memory of the functions' VM", "函数虚拟机占用的内存"},
	"used_memory_vm_total":          {"Memory of both VMs, in bytes", "两个虚拟机占用的内存合计（字节）"},
	"used_memory_vm_total_human":    {"The same, in K/M/G", "同上，以 K/M/G 显示"},
	"used_memory_functions":         {"Memory the functions take", "函数本身占用的内存"},
	"used_memory_scripts":           {"Memory the scripts take", "脚本占用的内存合计"},
	"used_memory_scripts_human":     {"The same, in K/M/G", "同上，以 K/M/G 显示"},
	"maxmemory":                     {"Limit it evicts at, 0 for none", "内存上限（字节），0 为不限"},
	"maxmemory_human":               {"The same, in K/M/G", "同上，以 K/M/G 显示"},
	"maxmemory_policy":              {"What it does at that limit, as noeviction", "达到上限时的策略，如 noeviction"},
	"allocator_frag_ratio":          {"Active pages of the allocated memory", "分配器的 active 与 allocated 之比"},
	"allocator_frag_bytes":          {"The difference between them", "两者相差的字节数"},
	"allocator_rss_ratio":           {"Resident pages of the active ones", "常驻内存与 active 之比"},
	"allocator_rss_bytes":           {"The difference between them", "两者相差的字节数"},
	"rss_overhead_ratio":            {"The server's RSS of the allocator's", "进程 RSS 与分配器常驻内存之比"},
	"rss_overhead_bytes":            {"The difference between them", "两者相差的字节数"},
	"mem_fragmentation_ratio":       {"RSS of the allocated memory", "RSS 与分配内存之比（碎片率）"},
	"mem_fragmentation_bytes":       {"The difference between them, in bytes", "两者的差值（字节）"},
	"mem_not_counted_for_evict":     {"Memory the eviction does not count", "不计入淘汰判断的内存"},
	"mem_replication_backlog":       {"Memory of the replication backlog", "复制积压缓冲占用的内存"},
	"mem_total_replication_buffers": {"Memory of every replication buffer", "所有复制缓冲占用的内存"},
	"mem_clients_slaves":            {"Memory of the replicas' buffers", "从节点连接缓冲占用的内存"},
	"mem_clients_normal":            {"Memory of the clients' buffers", "普通客户端缓冲占用的内存"},
	"mem_cluster_links":             {"Memory of the cluster links", "集群连接占用的内存"},
	"mem_aof_buffer":                {"Memory of the AOF buffer", "AOF 缓冲占用的内存"},
	"mem_allocator":                 {"Allocator it uses, as jemalloc", "使用的内存分配器，如 jemalloc"},
	"active_defrag_running":         {"Whether it defragments now", "是否正在做内存整理"},
	"lazyfree_pending_objects":      {"Objects waiting for a lazy free", "等待惰性删除的对象数"},
	"lazyfreed_objects":             {"Objects freed lazily so far", "已惰性删除的对象数"},

	// Persistence.
	"loading":                           {"Whether it loads a file at startup", "是否正在启动时加载数据文件"},
	"async_loading":                     {"Whether that loading is asynchronous", "该加载是否为异步"},
	"current_cow_peak":                  {"Largest copy on write of the running child", "当前子进程写时复制的峰值"},
	"current_cow_size":                  {"Copy on write of the running child", "当前子进程写时复制的字节数"},
	"current_cow_size_age":              {"Seconds since that was measured", "该数值的采样时间（秒前）"},
	"current_fork_perc":                 {"Progress of the running fork, in percent", "当前 fork 的进度（百分比）"},
	"current_save_keys_processed":       {"Keys the save has written", "本次保存已处理的键数"},
	"current_save_keys_total":           {"Keys the save has to write", "本次保存需要处理的键总数"},
	"rdb_changes_since_last_save":       {"Writes since the last RDB save", "自上次 RDB 以来的写次数"},
	"rdb_bgsave_in_progress":            {"Whether an RDB save runs now", "是否正在后台保存 RDB"},
	"rdb_last_save_time":                {"When it last saved, as a timestamp", "上次保存 RDB 的时间戳"},
	"rdb_last_bgsave_status":            {"How that save ended: ok or err", "上次保存结果：ok 或 err"},
	"rdb_last_bgsave_time_sec":          {"How long that save took, in seconds", "上次保存耗时（秒）"},
	"rdb_current_bgsave_time_sec":       {"How long the running one has taken", "当前保存已进行的时间（秒）"},
	"rdb_saves":                         {"RDB saves since it started", "启动以来的 RDB 保存次数"},
	"rdb_last_cow_size":                 {"Memory copied during that save", "上次保存时复制的内存"},
	"rdb_last_load_keys_expired":        {"Expired keys the last load dropped", "上次加载时丢弃的过期键数"},
	"rdb_last_load_keys_loaded":         {"Keys the last load read", "上次加载读入的键数"},
	"aof_enabled":                       {"Whether the AOF is on", "是否开启 AOF"},
	"aof_rewrite_in_progress":           {"Whether an AOF rewrite runs now", "是否正在重写 AOF"},
	"aof_rewrite_scheduled":             {"Whether a rewrite waits for the save", "是否有重写等待当前保存结束"},
	"aof_last_rewrite_time_sec":         {"How long the last rewrite took", "上次重写耗时（秒）"},
	"aof_current_rewrite_time_sec":      {"How long the running one has taken", "当前重写已进行的时间（秒）"},
	"aof_last_bgrewrite_status":         {"How it ended: ok or err", "上次重写结果：ok 或 err"},
	"aof_rewrites":                      {"Rewrites since it started", "启动以来的 AOF 重写次数"},
	"aof_rewrites_consecutive_failures": {"Rewrites that failed in a row", "连续失败的 AOF 重写次数"},
	"aof_last_write_status":             {"Whether the last write to it worked", "上次写入 AOF 的结果"},
	"aof_last_cow_size":                 {"Memory copied during the last rewrite", "上次重写时复制的内存"},
	"aof_base_size":                     {"Size of the AOF at the last rewrite", "上次重写时的 AOF 大小"},
	"aof_current_size":                  {"Size of the AOF now", "AOF 当前大小"},
	"aof_buffer_length":                 {"Bytes waiting in the AOF buffer", "AOF 缓冲区中待写入的字节数"},
	"aof_pending_bio_fsync":             {"Fsyncs the background IO owes the AOF", "后台 IO 待完成的 fsync 次数"},
	"aof_delayed_fsync":                 {"Fsyncs that had to wait", "被延迟的 fsync 次数"},
	"module_fork_in_progress":           {"Whether a module forks now", "模块是否正在 fork"},
	"module_fork_last_cow_size":         {"Memory its last fork copied", "模块上次 fork 复制的内存"},

	// Stats.
	"total_connections_received":                {"Connections it has taken since it started", "启动以来接受的连接总数"},
	"total_commands_processed":                  {"Commands it has run since it started", "启动以来执行的命令总数"},
	"instantaneous_ops_per_sec":                 {"Commands per second, over the last sample", "最近一次采样的每秒命令数"},
	"total_net_input_bytes":                     {"Bytes it has read since it started", "启动以来读取的字节数"},
	"total_net_output_bytes":                    {"Bytes it has written since it started", "启动以来写出的字节数"},
	"total_net_repl_input_bytes":                {"Bytes read from its replicas", "从从节点读取的字节数"},
	"total_net_repl_output_bytes":               {"Bytes written to them", "写给从节点的字节数"},
	"instantaneous_input_kbps":                  {"Kilobytes a second it reads", "当前每秒读取的 KB 数"},
	"instantaneous_output_kbps":                 {"Kilobytes a second it writes", "当前每秒写出的 KB 数"},
	"instantaneous_input_repl_kbps":             {"Kilobytes a second it reads from replicas", "当前每秒从从节点读取的 KB 数"},
	"instantaneous_output_repl_kbps":            {"Kilobytes a second it writes to them", "当前每秒写给从节点的 KB 数"},
	"rejected_connections":                      {"Connections refused at maxclients", "因达到最大连接数被拒绝的连接数"},
	"sync_full":                                 {"Full resynchronizations with replicas", "与从节点的全量同步次数"},
	"sync_partial_ok":                           {"Partial ones that worked", "成功的部分同步次数"},
	"sync_partial_err":                          {"Partial ones that failed", "失败的部分同步次数"},
	"expired_keys":                              {"Keys that expired so far", "已过期的键总数"},
	"expired_stale_perc":                        {"Expired keys still in memory, in percent", "已过期但仍在内存中的键占比"},
	"expired_time_cap_reached_count":            {"Times the expire cycle ran out of time", "过期清理因时间用尽而中断的次数"},
	"expire_cycle_cpu_milliseconds":             {"CPU time of the expire cycle, in ms", "过期清理占用的 CPU 时间（毫秒）"},
	"evicted_keys":                              {"Keys evicted at maxmemory", "因内存上限被淘汰的键数"},
	"evicted_clients":                           {"Clients dropped at maxmemory-clients", "因客户端内存上限被断开的连接数"},
	"total_eviction_exceeded_time":              {"Milliseconds memory stayed over the limit", "内存超出上限的总时长（毫秒）"},
	"current_eviction_exceeded_time":            {"Milliseconds it is over it now", "当前这次超出上限的时长（毫秒）"},
	"keyspace_hits":                             {"Lookups answered from memory", "在内存中找到键的次数"},
	"keyspace_misses":                           {"Lookups that found nothing", "未找到键的次数"},
	"pubsub_channels":                           {"Channels in use", "在用的频道数"},
	"pubsub_patterns":                           {"Patterns subscribed", "订阅的模式数"},
	"pubsubshard_channels":                      {"Shard channels in use", "在用的分片频道数"},
	"latest_fork_usec":                          {"Microseconds the last fork took", "上次 fork 的耗时（微秒）"},
	"total_forks":                               {"Forks so far, as for a save", "启动以来 fork 的次数"},
	"migrate_cached_sockets":                    {"Sockets kept for MIGRATE", "为 MIGRATE 缓存的连接数"},
	"slave_expires_tracked_keys":                {"Expiring keys the replicas track", "从节点跟踪的带过期时间的键数"},
	"active_defrag_hits":                        {"Values it moved to defragment", "内存整理中移动的值数"},
	"active_defrag_misses":                      {"Values it could not move", "内存整理中未能移动的值数"},
	"active_defrag_key_hits":                    {"Keys whose values it moved", "内存整理中移动过值的键数"},
	"active_defrag_key_misses":                  {"Keys it could not defragment", "内存整理未能处理的键数"},
	"total_active_defrag_time":                  {"Milliseconds spent defragmenting", "内存整理的总时长（毫秒）"},
	"current_active_defrag_time":                {"Milliseconds of the running pass", "当前这次整理的时长（毫秒）"},
	"tracking_total_keys":                       {"Keys the clients track", "客户端跟踪的键数"},
	"tracking_total_items":                      {"Items of those keys", "这些键中的元素数"},
	"tracking_total_prefixes":                   {"Prefixes tracked", "跟踪的前缀数"},
	"unexpected_error_replies":                  {"Errors it answered of its own fault", "服务器自身问题导致的错误回复数"},
	"total_error_replies":                       {"Error replies it has sent", "已发送的错误回复总数"},
	"dump_payload_sanitizations":                {"DUMP payloads it sanitized", "做过的 DUMP 载荷清理次数"},
	"total_reads_processed":                     {"Read events it handled", "处理的读事件数"},
	"total_writes_processed":                    {"Write events it handled", "处理的写事件数"},
	"io_threaded_reads_processed":               {"Reads the IO threads handled", "IO 线程处理的读事件数"},
	"io_threaded_writes_processed":              {"Writes they handled", "IO 线程处理的写事件数"},
	"reply_buffer_shrinks":                      {"Times the reply buffer shrank", "回复缓冲收缩的次数"},
	"reply_buffer_expands":                      {"Times it grew", "回复缓冲扩大的次数"},
	"eventloop_cycles":                          {"Event loop cycles so far", "事件循环的总轮数"},
	"eventloop_duration_sum":                    {"Microseconds the loop spent, in total", "事件循环累计耗时（微秒）"},
	"eventloop_duration_cmd_sum":                {"Microseconds it spent on commands", "处理命令累计耗时（微秒）"},
	"instantaneous_eventloop_cycles_per_sec":    {"Event loop cycles a second", "每秒事件循环轮数"},
	"instantaneous_eventloop_duration_usec":     {"Microseconds one cycle takes", "每轮事件循环平均耗时（微秒）"},
	"client_query_buffer_limit_disconnections":  {"Clients dropped for their query buffer", "因查询缓冲超限被断开的连接数"},
	"client_output_buffer_limit_disconnections": {"Clients dropped for their output buffer", "因输出缓冲超限被断开的连接数"},

	// Replication.
	"role":                           {"master or slave", "主节点（master）或从节点（slave）"},
	"connected_slaves":               {"Replicas connected to it", "连接着的从节点数"},
	"master_failover_state":          {"State of a failover, as no-failover", "故障转移状态，如 no-failover"},
	"master_replid":                  {"Identifier of its replication stream", "复制流标识"},
	"master_replid2":                 {"The identifier before the last failover", "上次故障转移前的复制流标识"},
	"master_repl_offset":             {"How far that stream has gone", "复制偏移量"},
	"second_repl_offset":             {"Second offset of the stream", "复制的第二个偏移量"},
	"repl_backlog_active":            {"Whether a backlog is kept", "是否保留复制积压缓冲"},
	"repl_backlog_size":              {"Size of that backlog", "积压缓冲大小"},
	"repl_backlog_first_byte_offset": {"Where the backlog starts", "积压缓冲的起始偏移"},
	"repl_backlog_histlen":           {"Bytes in the backlog", "积压缓冲中的字节数"},
	"master_host":                    {"Address of the master it follows", "所跟随主节点的地址"},
	"master_port":                    {"Port of that master", "主节点的端口"},
	"master_link_status":             {"Whether the replica reaches its master: up or down", "与主节点的连接状态：up 或 down"},
	"master_last_io_seconds_ago":     {"Seconds since the replica last heard its master", "距上次收到主节点数据的时间（秒）"},
	"master_sync_in_progress":        {"Whether the replica is syncing now", "从节点是否正在同步"},
	"slave_read_only":                {"Whether a replica refuses writes", "从节点是否只读"},
	"slave_repl_offset":              {"How far the replica has read", "从节点已读到的偏移量"},
	"slave_priority":                 {"Priority a sentinel weighs it by", "哨兵选择它时使用的优先级"},
	"slave_read_repl_offset":         {"How far it has read, for its own replicas", "它作为从节点已读到的偏移量"},
	"replica_announced":              {"Whether it announces itself to its master", "是否向主节点宣告自己"},

	// CPU.
	"used_cpu_sys":              {"System time the server used, in seconds", "服务器占用的内核态 CPU 时间（秒）"},
	"used_cpu_user":             {"User time it used, in seconds", "服务器占用的用户态 CPU 时间（秒）"},
	"used_cpu_sys_children":     {"System time of its children, as a save", "子进程（如保存）占用的内核态时间"},
	"used_cpu_user_children":    {"User time of them", "子进程占用的用户态时间"},
	"used_cpu_sys_main_thread":  {"System time of the main thread", "主线程占用的内核态时间"},
	"used_cpu_user_main_thread": {"User time of the main thread", "主线程占用的用户态时间"},

	// Cluster and Modules.
	"cluster_enabled": {"Whether cluster mode is on", "是否启用集群模式"},
}

// redisPrefixDocs are the fields whose name is the value's: a database, an
// error, a command, a module, a listener, a replica, or the latencies of a
// command.
var redisPrefixDocs = []struct {
	prefix string
	doc    redisDoc
}{
	{"db", redisDoc{"Keys, expiring keys and their average TTL of that database", "该数据库的键数、带过期时间的键数与平均 TTL"}},
	{"errorstat_", redisDoc{"Commands refused with that error", "以该错误被拒绝的命令数"}},
	{"cmdstat_", redisDoc{"Calls of that command, its time and errors", "该命令的调用次数、耗时与错误数"}},
	{"latency_percentiles_usec_", redisDoc{"Percentiles of that command's latency, in microseconds", "该命令延迟的百分位（微秒）"}},
	{"module", redisDoc{"A module loaded: its name, version, API and filters", "已加载的模块：名称、版本、API 与过滤条件"}},
	{"listener", redisDoc{"A listening socket: its name, address and port", "监听端口：名称、绑定地址与端口"}},
	{"slave", redisDoc{"A replica: its address, port, state and how far behind it is", "从节点：地址、端口、状态与延迟"}},
}

// redisInfoDoc is what field means, "" when the app does not know it.
func redisInfoDoc(field string) string {
	zh := strings.HasPrefix(i18n.Locale(), "zh")
	d, ok := redisFieldDocs[field]
	if !ok {
		for _, p := range redisPrefixDocs {
			if strings.HasPrefix(field, p.prefix) {
				d, ok = p.doc, true
				break
			}
		}
	}
	if !ok {
		return ""
	}
	if zh {
		return d.zh
	}
	return d.en
}
