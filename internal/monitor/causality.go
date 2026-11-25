package monitor

// The causal chain of a process, as QuickTerm traces it: the processes
// that started it, from init down, each with what it is, who runs it and
// how long it has run, and what the chain tells of the target. The server
// only gathers facts, all from /proc in one round trip; what they mean is
// said where they are shown.

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// CausalityNode is a process on the chain of another: the target at the
// bottom, and the processes that started it above.
type CausalityNode struct {
	PID  int
	PPID int
	User string
	// State is the letter of the process's stat: R, S, D, Z, T, …; 0 when
	// unknown.
	State byte
	// Age is how many seconds the process has run; 0 when unknown.
	Age uint64
	// RSS is its resident memory; 0 when unknown.
	RSS uint64
	// Kind says where it came from: "init", "ssh", "cron", "container",
	// "multiplexer", "shell", "orphan", or "proc" for the rest.
	Kind    string
	Command string
	// IsTarget marks the process whose chain this is.
	IsTarget bool
}

// CausalityWarning is something the chain says of the target, to be
// weighed rather than acted on. Level is "danger", "warn" or "info"; Code
// names the warning, which Arg, when it has one, tells more of: the days
// it ran, the megabytes it holds, the injected variables.
type CausalityWarning struct {
	Level string
	Code  string
	Arg   string
}

// Causality is one tracing of a process.
type Causality struct {
	PID int
	// Found is false when the chain could not be read, Reason saying why:
	// "gone" for a process that ended, "not-linux" for a server without
	// /proc, or "" when the server returned nothing.
	Found    bool
	Degraded bool // not root, and the chain holds another user's process
	// Nodes is from the oldest ancestor the walk reached down to the
	// target, which is the last.
	Nodes    []CausalityNode
	Warnings []CausalityWarning
	Reason   string
}

// causalityScript walks pid's parents: its stat, owner, age, memory, kind
// and command, then its parent's, up to init or the 24th ancestor. It is
// POSIX sh over /proc alone, as the process list is, so it runs with
// BusyBox's tools. Each line is a record of tab-separated fields:
//
//	FS_NODE   pid ppid user state age rss_kb kind command
//	FS_FACT   name value
//	FS_WARN   level code [arg]
//	FS_META   degraded
//	FS_REASON code
//	FS_END
//
// The kinds are read from the cgroup first, then the name, then whether
// init adopted the process; the warnings from the facts already gathered,
// with no guess of their own.
func causalityScript(pid int) string {
	return `export LC_ALL=C
P=` + strconv.Itoa(pid) + `
PROC=/proc
[ "$(uname -s)" = Linux ] || { printf 'FS_REASON\tnot-linux\nFS_END\n'; exit 0; }
[ -d "$PROC/$P" ] || { printf 'FS_REASON\tgone\nFS_END\n'; exit 0; }

stat_rest() { sed 's/^[^ ]* (.*) //' "$PROC/$1/stat" 2>/dev/null; }
field() { stat_rest "$1" | awk -v i="$2" '{print $i; exit}'; }

HZ=$(getconf CLK_TCK 2>/dev/null)
case "$HZ" in ''|*[!0-9]*) HZ=100;; esac
UP=$(cut -d. -f1 "$PROC/uptime" 2>/dev/null)
ME=$(id -u 2>/dev/null)
case "$ME" in ''|*[!0-9]*) ME=-1;; esac

uid_rss() {
  awk '/^Uid:/{u=$2} /^VmRSS:/{r=$2} END{if(u=="")u="-"; if(r=="")r="-"; print u, r}' \
    "$PROC/$1/status" 2>/dev/null
}
user_of() {
  case "$1" in ''|*[!0-9]*) echo '-'; return;; esac
  N=$(awk -F: -v u="$1" '$3==u{print $1; exit}' /etc/passwd 2>/dev/null)
  if [ -n "$N" ]; then echo "$N"; else echo "#$1"; fi
}
age_of() {
  [ -n "$UP" ] || { echo '-'; return; }
  S=$(field "$1" 20)
  case "$S" in ''|*[!0-9]*) echo '-'; return;; esac
  A=$(( UP - S / HZ ))
  [ "$A" -lt 0 ] && A=0
  echo "$A"
}
cmd_of() {
  C=$(tr '\0' ' ' < "$PROC/$1/cmdline" 2>/dev/null)
  C=${C% }
  if [ -n "$C" ]; then echo "$C"; return; fi
  N=$(cat "$PROC/$1/comm" 2>/dev/null)
  if [ -n "$N" ]; then echo "[$N]"; else echo '?'; fi
}
kind_of() {
  case "$(cat "$PROC/$1/cgroup" 2>/dev/null)" in
    *docker*|*kubepods*|*containerd*|*lxc*) echo container; return;;
  esac
  case "$(cat "$PROC/$1/comm" 2>/dev/null)" in
    systemd|init) echo init; return;;
    sshd|dropbear) echo ssh; return;;
    crond|cron) echo cron; return;;
    tmux*|screen*) echo multiplexer; return;;
    sh|bash|zsh|ash|dash|fish) echo shell; return;;
  esac
  [ "$2" = 1 ] && { echo orphan; return; }
  echo proc
}

CUR=$P
I=0
FIRST=''
TUID=''
TAGE=''
TRSS=''
TKIND=''
DEG=0
HAS_SSH=0
ADOPTED=0
REACHED1=0
while [ -n "$CUR" ] && [ "$CUR" != 0 ] && [ "$I" -lt 24 ]; do
  PP=$(field "$CUR" 2)
  ST=$(field "$CUR" 1)
  [ -z "$PP" ] && PP=0
  [ -z "$ST" ] && ST=?
  AC=$(age_of "$CUR")
  UR=$(uid_rss "$CUR")
  NU=${UR%% *}
  RS=${UR##* }
  KD=$(kind_of "$CUR" "$PP")
  printf 'FS_NODE\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$CUR" "$PP" "$(user_of "$NU")" "$ST" "$AC" "$RS" "$KD" "$(cmd_of "$CUR")"
  if [ "$CUR" = "$P" ]; then FIRST=$ST; TUID=$NU; TAGE=$AC; TRSS=$RS; TKIND=$KD; fi
  [ "$KD" = ssh ] && HAS_SSH=1
  [ "$CUR" = 1 ] && REACHED1=1
  [ "$PP" = 1 ] && [ "$CUR" != 1 ] && ADOPTED=1
  if [ "$NU" != "-" ] && [ "$ME" != "-1" ] && [ "$ME" != 0 ] && [ "$NU" != "$ME" ]; then
    DEG=1
  fi
  [ "$PP" = 0 ] && break
  [ "$PP" = "$CUR" ] && break
  CUR=$PP
  I=$(( I + 1 ))
done

CWD=$(readlink "$PROC/$P/cwd" 2>/dev/null)
printf 'FS_FACT\tcwd\t%s\n' "$CWD"
printf 'FS_FACT\texe\t%s\n' "$(readlink "$PROC/$P/exe" 2>/dev/null)"

case "$FIRST" in
  Z*) printf 'FS_WARN\tdanger\tzombie\n';;
  T*|t*) printf 'FS_WARN\twarn\tstopped\n';;
  D*) printf 'FS_WARN\twarn\tuninterruptible\n';;
esac
[ "$TUID" = 0 ] && printf 'FS_WARN\tdanger\troot\n'
case "$TAGE" in
  ''|*[!0-9]*) ;;
  *) [ "$TAGE" -ge 7776000 ] && printf 'FS_WARN\tinfo\tlong-running\t%s\n' "$(( TAGE / 86400 ))";;
esac
case "$TRSS" in
  ''|*[!0-9]*) ;;
  *) [ "$TRSS" -ge 1048576 ] && printf 'FS_WARN\twarn\tbig-rss\t%s\n' "$(( TRSS / 1024 ))";;
esac
[ "$TKIND" = container ] && printf 'FS_WARN\tinfo\tcontainer\n'
[ "$HAS_SSH" = 1 ] && printf 'FS_WARN\tinfo\tssh\n'
[ "$ADOPTED" = 1 ] && printf 'FS_WARN\tinfo\tadopted\n'
[ "$REACHED1" = 0 ] && printf 'FS_WARN\tinfo\tno-init\n'
[ "$DEG" = 1 ] && printf 'FS_WARN\tinfo\tnon-root\n'
if [ -z "$CWD" ] && [ "$TUID" != "$ME" ]; then
  printf 'FS_WARN\tinfo\tno-cwd\n'
fi
ENV=$(tr '\0' '\n' < "$PROC/$P/environ" 2>/dev/null | grep -E '^LD_(PRELOAD|LIBRARY_PATH)=' | head -n 2)
if [ -n "$ENV" ]; then
  printf 'FS_WARN\twarn\tinjected\t%s\n' "$(printf '%s' "$ENV" | tr '\n' ' ')"
fi

printf 'FS_META\t%s\n' "$DEG"
printf 'FS_END\n'
exit 0
`
}

// ReadCausality traces pid on client: one exec channel, one script.
func ReadCausality(ctx context.Context, client *ssh.Client, pid int) (Causality, error) {
	if pid <= 0 {
		return Causality{PID: pid, Reason: "gone"}, nil
	}
	out, _, err := RunScript(ctx, client, causalityScript(pid))
	if ctx.Err() != nil {
		return Causality{}, ctx.Err()
	}
	if !strings.Contains(out, "FS_END") {
		if err != nil {
			return Causality{}, err
		}
		return Causality{}, ErrScriptOutput
	}
	return parseCausality(pid, out), nil
}

// parseCausality reads the script's records. The nodes come as the walk
// made them, from the target up, and are put from the root down.
func parseCausality(pid int, out string) Causality {
	c := Causality{PID: pid}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(line, "FS_NODE\t"):
			f := strings.SplitN(line, "\t", 9)
			if len(f) < 9 {
				continue
			}
			n := CausalityNode{
				PID: atoiOr(f[1], 0), PPID: atoiOr(f[2], 0),
				User: f[3], Kind: f[7], Command: f[8],
			}
			if f[4] != "" && f[4] != "-" {
				n.State = f[4][0]
			}
			n.Age, _ = strconv.ParseUint(f[5], 10, 64)
			if kb, err := strconv.ParseUint(f[6], 10, 64); err == nil {
				n.RSS = kb * 1024
			}
			n.IsTarget = n.PID == pid
			c.Nodes = append(c.Nodes, n)
		case strings.HasPrefix(line, "FS_WARN\t"):
			f := strings.SplitN(line, "\t", 4)
			if len(f) < 3 || f[2] == "" {
				continue
			}
			w := CausalityWarning{Level: f[1], Code: f[2]}
			if len(f) > 3 {
				w.Arg = strings.TrimSpace(f[3])
			}
			c.Warnings = append(c.Warnings, w)
		case strings.HasPrefix(line, "FS_META\t"):
			c.Degraded = strings.TrimSpace(strings.TrimPrefix(line, "FS_META\t")) == "1"
		case strings.HasPrefix(line, "FS_REASON\t"):
			c.Reason = strings.TrimSpace(strings.TrimPrefix(line, "FS_REASON\t"))
		}
	}
	slices.Reverse(c.Nodes)
	c.Found = len(c.Nodes) > 0
	return c
}

// atoiOr reads a number, or def when s is not one.
func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}
