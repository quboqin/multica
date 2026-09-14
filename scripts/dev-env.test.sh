#!/usr/bin/env bash
# Registry-level behaviour of scripts/dev-env.sh, with no services started.
#
# Everything here runs against a throwaway MULTICA_DEV_HOME holding hand-written
# manifests, so the verbs are exercised end to end without a database, a
# backend, or a port.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

export MULTICA_DEV_HOME="$tmp_dir/dev"
export MULTICA_DEV_WORKSPACES_PARENT="$tmp_dir/workspaces-parent"
export MULTICA_DEV_DESKTOP_APP_DATA="$tmp_dir/app-data"
export MULTICA_DEV_PROFILES_HOME="$tmp_dir/profiles"

fake_bin="$tmp_dir/bin"
mkdir -p "$fake_bin"
cat > "$fake_bin/psql" <<'EOF'
#!/usr/bin/env bash
case " $* " in
  *" DROP DATABASE "*) [ "${FAIL_DROP:-0}" != 1 ] ;;
  *) printf '1\n' ;;
esac
EOF
chmod +x "$fake_bin/psql"
export PATH="$fake_bin:$PATH"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

require_contains() {
  local file=$1 expected=$2
  if ! grep -Fq "$expected" "$file"; then
    echo "Expected output to contain: $expected" >&2
    echo "Observed:" >&2
    sed 's/^/  /' "$file" >&2
    exit 1
  fi
}

dev_env() {
  bash "$root_dir/scripts/dev-env.sh" "$@"
}

write_manifest() {
  local name=$1 dir=$2 offset=$3
  local profile="dev-dev-env-test-$offset"
  mkdir -p "$MULTICA_DEV_HOME/envs/$name/logs"
  cat > "$MULTICA_DEV_HOME/envs/$name/manifest.env" <<EOF
NAME=$name
DIR=$(printf '%q' "$dir")
CREATED_AT=2026-01-01T00:00:00Z
OWNER=agent
TTL_HOURS=0
ENV_FILE=.env.example
OFFSET=$offset
BACKEND_PORT=$((18080 + offset))
FRONTEND_PORT=$((13000 + offset))
DB_NAME=multica_dev_env_test_$offset
DATABASE_URL=postgres://multica:multica@localhost:5432/multica_dev_env_test_$offset?sslmode=disable
PROFILE=$profile
WORKSPACES_ROOT=$(printf '%q' "$MULTICA_DEV_WORKSPACES_PARENT/multica_workspaces_$profile")
DESKTOP_RENDERER_PORT=$((5174 + offset))
DESKTOP_APP_SUFFIX=$name
EOF
}

out="$tmp_dir/out"

assert_listener_ownership() {
  local case_name=$1 expected=$2 launcher=$3 listener=$4 listener_pgid=$5 recorded=${6:-}
  (
    # shellcheck disable=SC1090
    source "$root_dir/scripts/dev-env.sh"
    STATE_DIR="$tmp_dir/ownership-$case_name"
    mkdir -p "$STATE_DIR"
    [ -z "$recorded" ] || printf '%s\n' "$recorded" > "$(listener_pid_file web)"
    TEST_LAUNCHER=$launcher
    TEST_LISTENER=$listener
    TEST_LISTENER_PGID=$listener_pgid
    TEST_CASE=$case_name

    component_pid() { printf '%s' "$TEST_LAUNCHER"; }
    port_listener_pid() { printf '%s' "$TEST_LISTENER"; }
    process_group_id() { printf '%s' "$TEST_LISTENER_PGID"; }
    process_parent_id() {
      case "$TEST_CASE:$1" in
        nested:420) printf '310' ;;
        nested:310) printf '200' ;;
        nested:200) printf '%s' "$TEST_LAUNCHER" ;;
        *) printf '1' ;;
      esac
    }

    local actual=external
    if listener_belongs_to_component web 13000; then actual=owned; fi
    [ "$actual" = "$expected" ] \
      || fail "$case_name listener ownership = $actual, want $expected"
  )
}

assert_nested_listener_is_recorded() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  STATE_DIR="$tmp_dir/ownership-record"
  mkdir -p "$STATE_DIR"

  component_pid() { printf '100'; }
  port_listener_pid() { printf '420'; }
  process_group_id() { printf '310'; }
  process_parent_id() {
    case "$1" in
      420) printf '310' ;;
      310) printf '200' ;;
      200) printf '100' ;;
      *) printf '1' ;;
    esac
  }

  local claimed
  claimed="$(record_component_listener web 13000)" \
    || fail "nested listener was not claimed"
  [ "$claimed" = 420 ] || fail "claimed listener = $claimed, want 420"
  [ "$(cat "$(listener_pid_file web)")" = 420 ] \
    || fail "nested listener pid was not recorded"
)

assert_stop_handles_listener() {
  local case_name=$1 listener=$2 listener_pgid=$3 listener_parent=$4 recorded=${5:-}
  local expected_target=${6:-}
  (
    # shellcheck disable=SC1090
    source "$root_dir/scripts/dev-env.sh"
    STATE_DIR="$tmp_dir/stop-$case_name"
    mkdir -p "$STATE_DIR"
    BACKEND_PORT=18080
    FRONTEND_PORT=13000
    DESKTOP_RENDERER_PORT=5174
    local signals="$STATE_DIR/signals"
    [ -z "$recorded" ] || printf '%s\n' "$recorded" > "$(listener_pid_file web)"
    TEST_CASE=$case_name
    TEST_LISTENER=$listener
    TEST_LISTENER_PGID=$listener_pgid
    TEST_LISTENER_PARENT=$listener_parent

    component_pid() { [ "$TEST_CASE" = nested ] && printf '100'; }
    port_listener_pid() { printf '%s' "$TEST_LISTENER"; }
    process_group_id() { printf '%s' "$TEST_LISTENER_PGID"; }
    process_parent_id() {
      case "$1" in
        "$TEST_LISTENER") printf '%s' "$TEST_LISTENER_PARENT" ;;
        "$TEST_LISTENER_PARENT") printf '100' ;;
        *) printf '1' ;;
      esac
    }
    sleep() { :; }
    kill() {
      [ "$1" != -0 ] || return 1
      printf 'signal=%s target=%s\n' "$1" "${2:-}" >> "$signals"
    }

    stop_component web > "$out" 2>&1 || fail "$case_name stop failed"
    if [ -n "$expected_target" ]; then
      require_contains "$signals" "target=$expected_target"
    elif [ -s "$signals" ]; then
      fail "$case_name stop signalled an external listener: $(cat "$signals")"
    fi
  )
}

assert_web_diagnostic_snapshot() {
  local case_name=$1 mock_listener=$2 recorded=$3 expected=$4 reason=$5
  (
    # shellcheck disable=SC1090
    source "$root_dir/scripts/dev-env.sh"
    STATE_DIR="$tmp_dir/diagnostic-$case_name"
    LOG_DIR="$STATE_DIR/logs"
    REPO_ROOT="$root_dir"
    DIR="$root_dir"
    NAME="diagnostic-$case_name"
    FRONTEND_PORT=13000
    mkdir -p "$STATE_DIR/logs"
    printf '100\n' > "$(pid_file web)"
    [ -z "$recorded" ] || printf '%s\n' "$recorded" > "$(listener_pid_file web)"
    TEST_CASE=$case_name

    lsof() {
      case " $* " in
        *" -iTCP:13000 "*)
          case "$TEST_CASE" in
            query-failure) printf 'lsof: permission denied\n' >&2; return 7 ;;
            no-listener) return 0 ;;
            foreign|listener-replaced) printf '%s\n' "$mock_listener" ;;
            *) printf '%s\n' "$mock_listener" ;;
          esac
          ;;
        *" -d cwd "*) printf 'n/tmp/diagnostic-cwd\n' ;;
        *" -d txt "*) printf 'n/tmp/diagnostic-exe\n' ;;
        *) return 1 ;;
      esac
    }
    ps() {
      case " $* " in
        *" -o pid= "*) printf '%s %s %s %s 00:01 128 test-process\n' "$1" 100 100 100 ;;
        *" -o ppid= "*) printf '100\n' ;;
        *) return 1 ;;
      esac
    }
    kill() {
      [ "$1" = -0 ] || return 1
      [ "${2:-}" != 100 ]
    }
    process_parent_id() {
      case "$TEST_CASE:$1" in
        launcher-exited:100) printf '1' ;;
        parent-disconnected:420) printf '1' ;;
        legal-parent-chain:420) printf '310' ;;
        legal-parent-chain:310) printf '200' ;;
        legal-parent-chain:200) printf '100' ;;
        legal-parent-chain:100) printf '1' ;;
        *) printf '1' ;;
      esac
    }

    local log
    log="$(diagnose_web_ownership_failure "$reason" "$FRONTEND_PORT")"
    [ -f "$log" ] || fail "$case_name did not create a diagnostic log"
    require_contains "$log" "schema=web-ownership-diagnostic.v1"
    require_contains "$log" "reason=$reason"
    require_contains "$log" "$expected"
    require_contains "$log" "port.requested=$FRONTEND_PORT"
    if grep -Fq 'MULTICA_TOKEN' "$log"; then
      fail "$case_name diagnostic leaked an environment variable"
    fi
  )
}

assert_web_failure_logs_before_stop() {
  local state_dir="$tmp_dir/diagnostic-order"
  local events="$state_dir/events"
  mkdir -p "$state_dir"
  (
    # shellcheck disable=SC1090
    source "$root_dir/scripts/dev-env.sh"
    STATE_DIR="$state_dir"
    LOG_DIR="$STATE_DIR/logs"
    REPO_ROOT="$root_dir"
    DIR="$root_dir"
    NAME=diagnostic-order
    FRONTEND_PORT=13000
    ENV_FILE=.env.test
    mkdir -p "$LOG_DIR"
    local calls=0

    curl() {
      calls=$((calls + 1))
      [ "$calls" -gt 1 ]
    }
    port_free() { return 0; }
    launch_detached() { printf '100\n' > "$(pid_file web)"; }
    record_component_listener() { return 1; }
    lsof() {
      case " $* " in
        *" -iTCP:13000 "*) printf '999\n' ;;
        *" -d cwd "*) printf 'n/tmp/diagnostic-cwd\n' ;;
        *" -d txt "*) printf 'n/tmp/diagnostic-exe\n' ;;
        *) return 1 ;;
      esac
    }
    ps() { printf '100 100 100 100 00:01 128 web\n'; }
    kill() { [ "$1" = -0 ] && return 0; return 1; }
    process_parent_id() { [ "$1" = 999 ] && printf '1' || printf '100'; }
    stop_component() {
      set -- "$LOG_DIR"/web-ownership.*
      [ -f "$1" ] || fail "stop_component ran before the diagnostic log was created"
      printf 'stopped\n' >> "$events"
    }

    # die exits this child, just as the real ownership failure does. Assertions
    # below run in the parent so the test can inspect the retained evidence.
    start_web > "$out" 2>&1 || true
  ) || true
  [ -f "$events" ] || fail "ownership failure did not reach the stop path"
  [ "$(cat "$events")" = stopped ] || fail "unexpected stop-path event"
  set -- "$state_dir/logs"/web-ownership.*
  [ -f "$1" ] || fail "ownership failure did not create a per-attempt log"
}

assert_web_diagnostic_logs_are_independent() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  STATE_DIR="$tmp_dir/diagnostic-independent"
  LOG_DIR="$STATE_DIR/logs"
  REPO_ROOT="$root_dir"
  DIR="$root_dir"
  NAME=diagnostic-independent
  FRONTEND_PORT=13000
  mkdir -p "$LOG_DIR"
  printf '100\n' > "$(pid_file web)"
  lsof() { printf '420\n'; }
  ps() { printf '420 100 100 128 web\n'; }
  process_parent_id() { [ "$1" = 420 ] && printf '100' || printf '1'; }
  local first second
  first="$(diagnose_web_ownership_failure first_failure "$FRONTEND_PORT")"
  second="$(diagnose_web_ownership_failure second_failure "$FRONTEND_PORT")"
  [ "$first" != "$second" ] || fail "two ownership failures reused one diagnostic log"
  [ -f "$first" ] && [ -f "$second" ] || fail "independent diagnostic log was lost"
  set -- "$LOG_DIR"/web-ownership.*
  [ "$#" -eq 2 ] || fail "expected two independent diagnostic logs, found $#"
)

# ---------------------------------------------------------------------------
# An empty registry is a normal state, not an error.
# ---------------------------------------------------------------------------
dev_env list > "$out" 2>&1 || fail "list on an empty registry must succeed"
require_contains "$out" "No environments registered"

dev_env list --json > "$out" 2>&1 || fail "list --json on an empty registry must succeed"
if [ "$(cat "$out")" != "[]" ]; then
  fail "list --json on an empty registry = $(cat "$out"), want []"
fi

# ---------------------------------------------------------------------------
# Manifest serialization and user-provided names are safe. A manifest is
# sourced by Bash, so values must be shell-escaped and a name must never be
# able to walk outside envs/ before destroy eventually runs rm -rf.
# ---------------------------------------------------------------------------
quoted="$tmp_dir/quoted.env"
dangerous='a path with spaces;$(touch should-not-exist)'
bash -c 'source "$1"; write_manifest_value DIR "$2"' _ "$root_dir/scripts/dev-env.sh" "$dangerous" > "$quoted"
loaded="$(bash -c 'source "$1"; printf %s "$DIR"' _ "$quoted")"
[ "$loaded" = "$dangerous" ] || fail "manifest value did not round-trip safely"
[ ! -e "$root_dir/should-not-exist" ] || fail "loading a manifest executed its value"

status=0
dev_env up --name ../../escape > "$out" 2>&1 || status=$?
[ "$status" -ne 0 ] || fail "up accepted a path-traversing environment name"
require_contains "$out" "Invalid environment name"

status=0
dev_env up --ttl nope > "$out" 2>&1 || status=$?
[ "$status" -ne 0 ] || fail "up accepted a non-numeric TTL"
require_contains "$out" "TTL must be a positive integer"

# Rewriting an allocated database name must preserve the existing connection
# endpoint, credentials and query parameters.
rewritten="$(bash -c 'source "$1"; database_url_with_name "$2" "$3"' _ \
  "$root_dir/scripts/dev-env.sh" \
  'postgres://dev:p%40ss@127.0.0.1:55432/old_db?sslmode=require&application_name=dev' \
  'new_db')"
[ "$rewritten" = 'postgres://dev:p%40ss@127.0.0.1:55432/new_db?sslmode=require&application_name=dev' ] \
  || fail "database URL rewrite changed more than the database name: $rewritten"

# ---------------------------------------------------------------------------
# A registered environment is visible to both renderings, and the JSON one
# parses — agents read it, so a stray log line in it is a broken contract.
# ---------------------------------------------------------------------------
write_manifest "probe-901" "$tmp_dir/checkout" 901
mkdir -p "$tmp_dir/checkout"

dev_env list > "$out" 2>&1 || fail "list must succeed with one environment"
require_contains "$out" "probe-901"
require_contains "$out" "18981"

dev_env status probe-901 --json > "$out" 2>&1 || fail "status --json must succeed"
node -e '
  const fs = require("fs");
  const payload = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
  if (payload.name !== "probe-901") throw new Error("name = " + payload.name);
  if (payload.backend_port !== 18981) throw new Error("backend_port = " + payload.backend_port);
  for (const key of ["api", "web", "daemon", "desktop"]) {
    if (!payload.components[key]) throw new Error("missing component " + key);
    if (payload.components[key].state !== "stopped") {
      throw new Error(key + " state = " + payload.components[key].state);
    }
  }
' "$out" || fail "status --json is not machine-readable"

# ---------------------------------------------------------------------------
# Stopping an environment that is not running is a no-op that SUCCEEDS.
#
# This is the regression that made `make down` exit 1 after reporting success:
# on bash 3.2 a command substitution whose function ends in a failing command
# aborts the whole script under `set -e`, and "no process is listening on this
# port" is that function's normal answer.
# ---------------------------------------------------------------------------
status=0
dev_env down probe-901 --components api,web > "$out" 2>&1 || status=$?
if [ "$status" -ne 0 ]; then
  echo "Observed:" >&2
  sed 's/^/  /' "$out" >&2
  fail "down on a stopped environment exited $status, want 0"
fi
require_contains "$out" "stopped"

# Commands launched through env-exec must not inherit the daemon-task identity
# hints that make human/profile CLI commands reject --profile.
write_manifest "clean-env-903" "$root_dir" 903
MULTICA_TASK_CONFIG_ROOT=/task/config \
MULTICA_TASK_WORKSPACES_ROOT=/task/workspaces \
MULTICA_WORKSPACES_ROOT=/owner/workspaces \
  dev_env exec clean-env-903 -- sh -c '
    test -z "${MULTICA_TASK_CONFIG_ROOT:-}" &&
    test -z "${MULTICA_TASK_WORKSPACES_ROOT:-}" &&
    test "$MULTICA_WORKSPACES_ROOT" = "$1"
  ' _ "$MULTICA_DEV_WORKSPACES_PARENT/multica_workspaces_dev-dev-env-test-903" \
  > "$out" 2>&1 || fail "env-exec leaked daemon task identity or owner workspaces root"

# A health response without process identity is never proof that the process is
# this checkout's freshly launched API.
if bash -c 'source "$1"; api_started_after '\''{"status":"ok"}'\'' 1' _ "$root_dir/scripts/dev-env.sh"; then
  fail "legacy /health without started_at was accepted as current"
fi

# Listener ownership follows the process tree, not only the launcher's process
# group. Turbo/pnpm can create a nested process group for Next while keeping the
# listener below the launcher in the PPID chain.
assert_listener_ownership same-pgid owned 100 200 100
assert_listener_ownership nested owned 100 420 310
assert_listener_ownership recorded owned 100 200 200 200
assert_listener_ownership external external 100 999 999
assert_nested_listener_is_recorded

# Web ownership failures are observable without changing their verdict. Each
# case gets a separate log containing only the allowlisted identity/port/tree
# and resource fields.
assert_web_diagnostic_snapshot legal-parent-chain 420 "" \
  'parent_chain.listener=420->310->200->100->1' legal_parent_chain
assert_web_diagnostic_snapshot foreign 999 888 \
  'listener.replaced=1' foreign_port
assert_web_diagnostic_snapshot parent-disconnected 420 420 \
  'parent_chain.listener=420->1' parent_chain_disconnected
assert_web_diagnostic_snapshot launcher-exited 420 "" \
  'process.launcher.live=0' launcher_exited
assert_web_diagnostic_snapshot listener-replaced 999 420 \
  'listener.replaced=1' listener_replaced
assert_web_diagnostic_snapshot query-failure "" "" \
  'query.port.listeners.status=7' query_failure
assert_web_diagnostic_snapshot no-listener "" "" \
  'port.listeners=none' no_listener
assert_web_diagnostic_logs_are_independent
assert_web_failure_logs_before_stop

# Stopping first records an owned nested listener before killing the launcher's
# process group. An unrelated port occupant never receives a signal.
assert_stop_handles_listener nested 420 310 200 "" 420
assert_stop_handles_listener external 999 999 1 888

# ---------------------------------------------------------------------------
# Unknown names and components fail loudly instead of doing something else.
# ---------------------------------------------------------------------------
status=0
dev_env status no-such-env > "$out" 2>&1 || status=$?
[ "$status" -ne 0 ] || fail "status on an unknown environment must fail"
require_contains "$out" "Unknown environment"

status=0
dev_env up --components nope > "$out" 2>&1 || status=$?
[ "$status" -ne 0 ] || fail "up with an unknown component must fail"
require_contains "$out" "Unknown component"

# ---------------------------------------------------------------------------
# gc reports what it would collect and touches nothing in --dry-run. An
# environment whose checkout is gone has no owner left to stop it, which is how
# 152 databases accumulated with nothing on the machine able to list them.
# ---------------------------------------------------------------------------
write_manifest "orphan-902" "$tmp_dir/deleted-checkout" 902

dev_env gc --dry-run > "$out" 2>&1 || fail "gc --dry-run must succeed"
require_contains "$out" "orphan-902 would be collected"
if grep -Fq "probe-901 would be collected" "$out"; then
  fail "gc must not collect an environment whose directory still exists"
fi
[ -f "$MULTICA_DEV_HOME/envs/orphan-902/manifest.env" ] || fail "gc --dry-run deleted a manifest"

# A failed database drop keeps the manifest and slot so cleanup can be retried;
# destroy must never print success and forget the only deletion recipe.
write_manifest "drop-fails-904" "$root_dir" 904
status=0
FAIL_DROP=1 dev_env destroy drop-fails-904 --yes > "$out" 2>&1 || status=$?
[ "$status" -ne 0 ] || fail "destroy succeeded after DROP DATABASE failed"
[ -f "$MULTICA_DEV_HOME/envs/drop-fails-904/manifest.env" ] \
  || fail "destroy discarded the manifest after DROP DATABASE failed"
require_contains "$out" "manifest and slot were kept"
dev_env destroy drop-fails-904 --yes > "$out" 2>&1 || fail "retrying destroy after database recovery failed"

# ---------------------------------------------------------------------------
# destroy consumes the manifest: the slot is free afterwards, which is what
# makes the registry an allocator rather than a second place to leak.
# ---------------------------------------------------------------------------
dev_env destroy probe-901 --yes > "$out" 2>&1 || fail "destroy must succeed"
[ ! -d "$MULTICA_DEV_HOME/envs/probe-901" ] || fail "destroy left the environment directory behind"

dev_env list > "$out" 2>&1 || fail "list must succeed after destroy"
if grep -Fq "probe-901" "$out"; then
  fail "destroyed environment is still listed"
fi

# Declining the confirmation is a successful no-op, not a failure.
printf 'n\n' | dev_env destroy orphan-902 > "$out" 2>&1 || fail "declining destroy must exit 0"
require_contains "$out" "Cancelled."
[ -d "$MULTICA_DEV_HOME/envs/orphan-902" ] || fail "declined destroy removed the environment anyway"

echo "✓ dev-env.sh registry behaviour verified"
