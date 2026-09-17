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
cat > "$fake_bin/make" <<'EOF'
#!/usr/bin/env bash
case " $* " in
  *" web-dev "*)
    [ "${TEST_WEB_CASE:-}" = launcher-exited ] && exit 0
    sleep 0.2
    exit 0
    ;;
  *) exit 0 ;;
esac
EOF
chmod +x "$fake_bin/make"
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
    if [ "$case_name" = nested ]; then
      [ "$STOP_COMPONENT_LAST_LAUNCHER" = 100 ] \
        || fail "$case_name did not retain the non-empty launcher before signaling"
      while IFS= read -r signal_line; do
        signal_target="${signal_line#*target=}"
        case "$signal_target" in
          -100|420) ;;
          *) fail "$case_name signalled outside its exact allowlist: ${signal_line}" ;;
        esac
      done < "$signals"
    else
      [ -z "$STOP_COMPONENT_LAST_LAUNCHER" ] \
        || fail "$case_name unexpectedly retained a launcher"
    fi
    if [ -n "$expected_target" ]; then
      require_contains "$signals" "target=$expected_target"
    elif [ -s "$signals" ]; then
      fail "$case_name stop signalled an external listener: $(cat "$signals")"
    fi
  )
}

assert_stop_restores_same_pgid_listener() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  STATE_DIR="$tmp_dir/stop-late-same-pgid"
  mkdir -p "$STATE_DIR"
  FRONTEND_PORT=13000
  local signals="$STATE_DIR/signals"
  LAUNCHER_STOPPED=0
  LISTENER_STOPPED=0

  component_pid() { printf '100'; }
  port_listener_pid() {
    [ "$LAUNCHER_STOPPED" -eq 1 ] && printf '420'
  }
  process_group_id() {
    [ "$1" = 420 ] && printf '100'
  }
  sleep() { :; }
  kill() {
    local signal=$1 target=$2
    if [ "$signal" = -0 ]; then
      case "$target" in
        100) [ "$LAUNCHER_STOPPED" -eq 0 ] ;;
        420) [ "$LISTENER_STOPPED" -eq 0 ] ;;
        *) return 1 ;;
      esac
      return
    fi
    printf 'signal=%s target=%s\n' "$signal" "$target" >> "$signals"
    [ "$target" != -100 ] || LAUNCHER_STOPPED=1
    [ "$target" != 420 ] || LISTENER_STOPPED=1
    return 0
  }

  stop_component web > "$out" 2>&1 \
    || fail "late same-PGID listener stop failed"
  require_contains "$signals" "signal=-TERM target=-100"
  require_contains "$signals" "signal=-TERM target=420"
  while IFS= read -r signal_line; do
    case "${signal_line#*target=}" in
      -100|420) ;;
      *) fail "late same-PGID stop signalled outside its exact allowlist: $signal_line" ;;
    esac
  done < "$signals"
)

assert_signal_allowlists_default_deny() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  local signals="$tmp_dir/allowlist-signals"
  kill() {
    printf 'signal=%s target=%s\n' "$1" "$2" >> "$signals"
    return 0
  }

  DIAGNOSTIC_ALLOWED_TARGETS=""
  ! diagnostic_signal_allowed TERM 777 \
    || fail "diagnostic allowlist accepted an unknown positive PID"
  ! diagnostic_signal_allowed TERM -777 \
    || fail "diagnostic allowlist accepted an unknown negative PGID"
  diagnostic_allow_signal_target 123
  diagnostic_allow_signal_target -123
  diagnostic_signal_allowed TERM 123
  diagnostic_signal_allowed KILL -123

  STOP_COMPONENT_ALLOWED_TARGETS=""
  ! stop_component_signal TERM 777 >/dev/null 2>&1 \
    || fail "stop allowlist accepted an unknown positive PID"
  ! stop_component_signal TERM -777 >/dev/null 2>&1 \
    || fail "stop allowlist accepted an unknown negative PGID"
  stop_component_allow_target 100
  stop_component_allow_target -100
  stop_component_signal TERM 100
  stop_component_signal KILL -100

  [ "$(wc -l < "$signals" | tr -d ' ')" = 4 ] \
    || fail "allowlist probe sent an unexpected number of signals"
  if grep -Eq 'target=-?777$' "$signals"; then
    fail "an unallowlisted signal reached kill"
  fi
)

assert_web_start_case() {
  local case_name=$1 expected_status=$2 expected_listener=$3 expected_log=$4
  local state_dir="$tmp_dir/start-$case_name"
  local signals="$state_dir/signals" events="$state_dir/events" output="$state_dir/output"
  local started_epoch ended_epoch elapsed launcher_pid
  mkdir -p "$state_dir/logs"
  started_epoch="$(date +%s)"
  (
    # Only OS queries, process signals, and clock sleeps are replaced.
    # start_web, ownership, registration, diagnostics, and stop_component stay
    # real so this matrix covers the complete decision and cleanup chain.
    # shellcheck disable=SC1090
    source "$root_dir/scripts/dev-env.sh"
    eval "$(declare -f stop_component | sed '1s/^stop_component /real_stop_component /')"
    STATE_DIR="$state_dir"
    LOG_DIR="$STATE_DIR/logs"
    REPO_ROOT="$root_dir"
    DIR="$root_dir"
    NAME="start-$case_name"
    FRONTEND_PORT=13000
    ENV_FILE=.env.test
    case "$case_name" in
      parent-disconnected|parent-timeout|parent-failure) DIAGNOSTIC_TOTAL_TIMEOUT_SECONDS=5 ;;
    esac
    TEST_CASE=$case_name
    TEST_LISTENER=
    LAUNCHER_STOPPED=0
    CURL_CALLS=0
    export TEST_WEB_CASE=$case_name
    mkdir -p "$LOG_DIR"
    stop_component() {
      local saved_launcher
      saved_launcher="$(cat "$(pid_file "$1")" 2>/dev/null || true)"
      printf '%s\n' "$saved_launcher" > "$state_dir/launcher-before-stop"
      real_stop_component "$@"
    }
    if [ "$case_name" = recorded-pid ]; then
      TEST_LAUNCHER=100
      printf '%s\n' "$TEST_LAUNCHER" > "$(pid_file web)"
      printf '420\n' > "$(listener_pid_file web)"
    elif [ "$case_name" = listener-replaced ]; then
      printf '420\n' > "$(listener_pid_file web)"
    fi
    if [ "$case_name" = log-write-failure ]; then
      mktemp() {
        local created
        created="$(command mktemp "$@")" || return
        case "${1:-}" in
          "$LOG_DIR/web-ownership."*)
            rm -f "$created"
            mkdir "$created"
            ;;
        esac
        printf '%s\n' "$created"
      }
    fi

    curl() {
      CURL_CALLS=$((CURL_CALLS + 1))
      if [ "$case_name" = log-create-failure ] && [ "$CURL_CALLS" -gt 1 ]; then
        rm -rf "$LOG_DIR"
        : > "$LOG_DIR"
      fi
      if [ "$case_name" = recorded-pid ] || [ "$CURL_CALLS" -gt 1 ]; then
        return 0
      fi
      return 1
    }
    port_listener_pid() {
      [ -f "$(pid_file web)" ] || return 0
      case "$TEST_CASE" in
        no-listener|query-failure) return 0 ;;
        *) printf '%s\n' "$TEST_LISTENER" ;;
      esac
    }
    process_group_id() {
      case "$TEST_CASE:$1" in
        legal-nested:420) printf '310\n' ;;
        recorded-pid:420) printf '999\n' ;;
        foreign-negative-pgid:999) printf -- '-999\n' ;;
        pgid-budget:999) printf '999\n' ;;
        pgid-budget:*) command sleep 4; printf '999\n' ;;
        *) command ps -p "$1" -o pgid= 2>/dev/null | tr -d ' ' ;;
      esac
    }
    process_parent_id() {
      case "$TEST_CASE:$1" in
        legal-nested:420) printf '310\n' ;;
        legal-nested:310) cat "$(pid_file web)" ;;
        legal-nested:*) printf '1\n' ;;
        parent-timeout:999) printf '1\n' ;;
        parent-failure:999) return 7 ;;
        recorded-pid:420) printf '1\n' ;;
        *) printf '1\n' ;;
      esac
    }
    ps() {
      local pid=$2
      case " $* " in
        *" -o pid= "*)
          printf '%s %s %s 128 web\n' "$pid" "$pid" "$pid"
          if [ "$TEST_CASE" = long-output ]; then
            i=0
            while [ "$i" -lt 6000 ]; do printf x; i=$((i + 1)); done
            printf '\n'
          fi
          ;;
        *" -o ppid= "*)
          if [ "$case_name" = parent-timeout ] && [ "$pid" = 999 ]; then
            while :; do :; done
          fi
          if [ "$case_name" = parent-failure ] && [ "$pid" = 999 ]; then
            return 7
          fi
          process_parent_id "$pid"
          ;;
        *" -o pgid= "*) process_group_id "$pid" ;;
        *" -o sess="*|*" -o sid="*) printf '1\n' ;;
        *" -o lstart="*) printf 'Mon Jan 1 00:00:00 2026\n' ;;
        *) return 1 ;;
      esac
    }
    lsof() {
      case " $* " in
        *" -iTCP:13000 "*)
          printf 'diagnostic-query\n' >> "$events"
          case "$TEST_CASE" in
            query-failure)
              printf 'lsof: permission denied Authorization: Bearer TOPSECRET\n' >&2
              return 7
              ;;
            no-listener) return 0 ;;
            timeout) while :; do :; done ;;
            multi-listener)
              printf '999\n888\n999\nnot-a-pid\n'
              printf 'warning Authorization: Bearer TOPSECRET\n' >&2
              ;;
            long-output)
              printf '999\n'
              i=0
              while [ "$i" -lt 6000 ]; do printf x; i=$((i + 1)); done
              printf '\n'
              i=0
              while [ "$i" -lt 6000 ]; do printf s >&2; i=$((i + 1)); done
              printf ' warning Authorization: Bearer TOPSECRET\n' >&2
              ;;
            *)
              printf '%s\n' "$TEST_LISTENER"
              printf 'warning Authorization: Bearer TOPSECRET\n' >&2
              ;;
          esac
          ;;
        *" -d cwd "*) printf 'n/tmp/diagnostic-cwd\033[31m\n' ;;
        *" -d txt "*) printf 'n/tmp/diagnostic-exe\n' ;;
        *) return 1 ;;
      esac
    }
    kill() {
      local signal=$1 target=$2
      if [ "${DIAGNOSTIC_CLEANUP_ACTIVE:-0}" -eq 1 ]; then
        case " $DIAGNOSTIC_ALLOWED_TARGETS " in
          *" $target "*) ;;
          *) printf 'scope=diagnostic-denied signal=%s target=%s\n' "$signal" "$target" >> "$signals"; return 1 ;;
        esac
        printf 'scope=diagnostic signal=%s target=%s\n' "$signal" "$target" >> "$signals"
        command kill "$signal" "$target" 2>/dev/null || true
        return 0
      fi
      if [ "$signal" = -0 ]; then
        case "$TEST_CASE:$target" in
          launcher-exited:*) return 1 ;;
          *:100|*:420|*:888|*:999) return 0 ;;
        esac
        if [ -f "$(pid_file web)" ] \
          && [ "$target" = "$(cat "$(pid_file web)")" ]; then
          if [ "$LAUNCHER_STOPPED" -eq 0 ]; then return 0; fi
          return 1
        fi
        command kill -0 "$target" 2>/dev/null
        return $?
      fi
      printf 'scope=stop signal=%s target=%s\n' "$signal" "$target" >> "$signals"
      case "$target" in -*) printf 'signal=%s target=%s\n' "$signal" "$target" >> "$events" ;; esac
      case "$target" in
        -*) LAUNCHER_STOPPED=1; return 0 ;;
        420|888|999) return 0 ;;
        *) command kill "$signal" "$target" 2>/dev/null || true ;;
      esac
    }
    sleep() { :; }
    case "$case_name" in
      legal-nested|recorded-pid) TEST_LISTENER=420 ;;
      multi-listener|timeout|parent-timeout|parent-failure|pgid-budget|foreign-negative-pgid|log-create-failure|log-write-failure) TEST_LISTENER=999 ;;
      foreign-positive) TEST_LISTENER=777 ;;
      foreign-negative) TEST_LISTENER=-777 ;;
      query-failure|no-listener) TEST_LISTENER= ;;
      *) TEST_LISTENER=999 ;;
    esac
    # start_web terminates through die(1) on these failures. Keep that exit in
    # a child so this parent can collect the real status and the stop evidence.
    set +e
    ( start_web >"$output" 2>&1 )
    status=$?
    set -e
    if [ "$status" -eq 0 ]; then
      [ ! -s "$signals" ] || {
        echo "start_web sent a signal before the assertion" >&2
        exit 1
      }
    fi
    printf 'status=%s\nlauncher=%s\n' "$status" \
      "$(cat "$state_dir/launcher-before-stop" 2>/dev/null || true)" > "$state_dir/result"
  )
  [ -f "$state_dir/result" ] || fail "$case_name did not preserve the start_web result"
  local status
  status="$(sed -n 's/^status=//p' "$state_dir/result")"
  launcher_pid="$(sed -n 's/^launcher=//p' "$state_dir/result")"
  chmod u+rwx "$state_dir/logs"
  ended_epoch="$(date +%s)"
  elapsed=$((ended_epoch - started_epoch))
  [ "$status" = "$expected_status" ] \
    || fail "$case_name start_web exit=$status, want $expected_status"
  if [ "$expected_status" = 0 ]; then
    if [ "$case_name" = recorded-pid ]; then
      require_contains "$output" "web already running on :13000 (pid 420)"
    else
      require_contains "$output" "web serving http://localhost:13000 (pid $expected_listener)"
    fi
    [ -f "$state_dir/web.listener.pid" ] \
      || fail "$case_name did not register a listener"
    [ "$(cat "$state_dir/web.listener.pid")" = "$expected_listener" ] \
      || fail "$case_name registered $(cat "$state_dir/web.listener.pid"), want $expected_listener"
    [ ! -s "$signals" ] || fail "$case_name sent a stop signal on success: $(cat "$signals")"
  else
    case "$launcher_pid" in
      ""|0|*[!0-9]*) fail "$case_name did not save the launcher identity before stop" ;;
    esac
    if grep -Fq "web serving" "$output" || grep -Fq "web already running" "$output"; then
      fail "$case_name reported ready after an ownership failure"
    fi
    [ ! -f "$state_dir/web.listener.pid" ] \
      || fail "$case_name retained a listener registration after stop"
    if [ "$case_name" = log-create-failure ] || [ "$case_name" = log-write-failure ]; then
      set -- "$state_dir/logs"/web-ownership.*
      if [ "$case_name" = log-create-failure ]; then
        [ "$#" -eq 1 ] && [ ! -e "$1" ] \
          || fail "$case_name unexpectedly created a diagnostic log"
      else
        [ "$#" -eq 1 ] && [ -d "$1" ] \
          || fail "$case_name did not retain the unwritable diagnostic target"
      fi
      require_contains "$signals" "signal=-TERM target=-$launcher_pid"
    else
      set -- "$state_dir/logs"/web-ownership.*
      [ "$#" -eq 1 ] || fail "$case_name did not retain exactly one independent diagnostic log"
      require_contains "$1" "$expected_log"
      if [ "$case_name" != timeout ] && [ "$case_name" != no-listener ]; then
        require_contains "$1" "query.port.listeners.stderr=present"
      fi
      if grep -Fq "TOPSECRET" "$1" || grep -Fq "permission denied Authorization" "$1"; then
        fail "$case_name exposed raw diagnostic error text"
      fi
      if grep -Fq $'\033' "$1"; then
        fail "$case_name retained a control character in diagnostic output"
      fi
      if [ "$case_name" = long-output ]; then
        require_contains "$1" "query.port.listeners.truncated=1"
        require_contains "$1" "query.port.listeners.stderr_truncated=1"
        require_contains "$1" "query.port.listeners.capture_bytes=6005"
        require_contains "$1" "query.port.listeners.capture_limit=4096"
        require_contains "$1" "port.listener.byte_truncated=1"
        require_contains "$1" "port.listener.enumeration_complete=0"
        require_contains "$1" "port.listener.count_truncated=0"
        require_contains "$1" "port.listener.count=1"
        require_contains "$1" "port.listener.total=unknown"
        if grep -Fq 'port.listener.2.pid=' "$1"; then
          fail "$case_name parsed a partial PID as an additional listener"
        fi
      fi
      case "$case_name" in
        query-failure|timeout|foreign-negative)
          require_contains "$1" "port.listener.status=unknown"
          require_contains "$1" "port.listener.enumeration_complete=0"
          require_contains "$1" "port.listeners=unknown"
          require_contains "$1" "port.listener.count=unknown"
          require_contains "$1" "port.listener.total=unknown"
          ! grep -Fq 'port.listener.1.pid=' "$1" \
            || fail "$case_name exposed an untrusted listener PID"
          ;;
        no-listener)
          require_contains "$1" "port.listener.status=known"
          require_contains "$1" "port.listener.enumeration_complete=1"
          require_contains "$1" "port.listener.total=0"
          ;;
      esac
      require_contains "$1" "event=ownership_check_failed"
      diag_line="$(grep -n '^diagnostic-query$' "$events" | cut -d: -f1 | head -1 || true)"
      stop_line="$(grep -n '^signal=-TERM target=-' "$events" | cut -d: -f1 | head -1 || true)"
      if [ "$case_name" = launcher-exited ]; then
        [ -n "$diag_line" ] && [ -z "$stop_line" ] \
          || fail "$case_name signalled after the launcher was already gone"
      else
        [ -n "$diag_line" ] && [ -n "$stop_line" ] && [ "$diag_line" -lt "$stop_line" ] \
          || fail "$case_name stop signal was not ordered after diagnostics"
        require_contains "$signals" "signal=-TERM target=-$launcher_pid"
      fi
      if [ "$case_name" = parent-timeout ]; then
        [ "$elapsed" -le 5 ] || fail "$case_name blocked for ${elapsed}s beyond the diagnostic budget"
        require_contains "$1" "query.parent.listener.999.error=timeout"
        require_contains "$1" "parent_chain.listener.complete=0"
      fi
      if [ "$case_name" = pgid-budget ]; then
        [ "$elapsed" -le 3 ] || fail "$case_name blocked for ${elapsed}s on a PGID lookup"
      fi
      if [ "$case_name" = parent-failure ]; then
        require_contains "$1" "query.parent.listener.999.error=command_failed"
        require_contains "$1" "parent_chain.listener=999->?"
        require_contains "$1" "parent_chain.listener.complete=0"
      fi
      if [ "$case_name" = parent-disconnected ]; then
        require_contains "$1" "parent_chain.listener=999->1"
        require_contains "$1" "parent_chain.listener.complete=1"
      fi
    fi
    if [ -s "$signals" ]; then
      while IFS= read -r signal_line; do
        signal_target="${signal_line#*target=}"
        case "$signal_line" in
          scope=diagnostic-denied*) fail "$case_name attempted an unallowlisted diagnostic signal: ${signal_line}" ;;
          scope=diagnostic*) ;;
          scope=stop*)
            case "$signal_target" in
              "-$launcher_pid"|"$launcher_pid") ;;
              *) fail "$case_name signalled outside the stop allowlist: ${signal_line}" ;;
            esac
            ;;
          *) fail "$case_name recorded an unclassified signal: ${signal_line}" ;;
        esac
      done < "$signals"
    fi
  fi
}

assert_pid_parser_boundaries() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  local raw="" i

  raw=$'101\n101\nbad-pid\n202\n'
  diagnostic_parse_pid_list "$raw" >/dev/null
  [ "$DIAGNOSTIC_PID_LIST" = $'101\n202' ] \
    || fail "PID parser did not deduplicate or skip invalid lines"
  [ "$DIAGNOSTIC_PID_COUNT" -eq 2 ] \
    || fail "PID parser count = $DIAGNOSTIC_PID_COUNT, want 2"
  [ "$DIAGNOSTIC_PID_INVALID_COUNT" -eq 1 ] \
    || fail "PID parser invalid count = $DIAGNOSTIC_PID_INVALID_COUNT, want 1"
  [ "$DIAGNOSTIC_PID_INPUT_INCOMPLETE" -eq 0 ] \
    || fail "complete PID input was marked incomplete"

  raw=$'303\n404'
  diagnostic_parse_pid_list "$raw" >/dev/null
  [ "$DIAGNOSTIC_PID_LIST" = 303 ] \
    || fail "PID parser retained an unterminated PID fragment"
  [ "$DIAGNOSTIC_PID_INPUT_INCOMPLETE" -eq 1 ] \
    || fail "unterminated PID input was not marked incomplete"

  raw=""
  i=1
  while [ "$i" -le 65 ]; do
    raw="$raw$i"$'\n'
    i=$((i + 1))
  done
  diagnostic_parse_pid_list "$raw" >/dev/null
  [ "$DIAGNOSTIC_PID_COUNT" -eq 64 ] \
    || fail "PID parser count = $DIAGNOSTIC_PID_COUNT, want bounded 64"
  [ "$DIAGNOSTIC_PID_INPUT_TRUNCATED" -eq 1 ] \
    || fail "PID parser did not mark the 65th unique PID as truncated"
  case "$DIAGNOSTIC_PID_LIST" in
    *$'\n65'*) fail "PID parser emitted a PID beyond the display bound" ;;
  esac
)

assert_diagnostic_logs_are_unique() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  STATE_DIR="$tmp_dir/repeated-diagnostics"
  LOG_DIR="$STATE_DIR/logs"
  mkdir -p "$LOG_DIR"
  lsof() { printf '999\n'; }
  diagnose_web_ownership_failure first 13000 >/dev/null
  diagnose_web_ownership_failure second 13000 >/dev/null
  set -- "$LOG_DIR"/web-ownership.*
  [ "$#" -eq 2 ] || fail "repeated diagnostics overwrote a log: found $#"
  for log in "$@"; do
    require_contains "$log" "event=ownership_check_failed"
  done
)

assert_diagnostic_enum_resets_between_calls() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  STATE_DIR="$tmp_dir/enum-reset"
  LOG_DIR="$STATE_DIR/logs"
  mkdir -p "$LOG_DIR"
  ENUM_MODE=success
  lsof() {
    case "$ENUM_MODE:$*" in
      success:*"-iTCP:13000"*) printf '111\n222\n' ;;
      failure:*"-iTCP:13000"*) return 7 ;;
      *) return 1 ;;
    esac
  }

  first_log="$(diagnose_web_ownership_failure first 13000)"
  ENUM_MODE=failure
  second_log="$(diagnose_web_ownership_failure second 13000)"
  set -- "$LOG_DIR"/web-ownership.*
  [ "$#" -eq 2 ] || fail "consecutive diagnostics did not retain two logs"
  require_contains "$first_log" "port.listener.status=known"
  require_contains "$first_log" "port.listener.total=2"
  require_contains "$second_log" "query.port.listeners.error=command_failed"
  require_contains "$second_log" "port.listener.status=unknown"
  require_contains "$second_log" "port.listeners=unknown"
  require_contains "$second_log" "port.listener.count=unknown"
  require_contains "$second_log" "port.listener.total=unknown"
  ! grep -Fq 'port.listener.1.pid=' "$second_log" \
    || fail "failed enumeration reused a PID from the prior call"
)

assert_capture_failure_reports_unknown() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  STATE_DIR="$tmp_dir/capture-failure"
  LOG_DIR="$STATE_DIR/logs"
  mkdir -p "$LOG_DIR"
  # Consume the FIFO normally, then fail the collector after writing its
  # bounded output. This exercises the same state path as a collector I/O
  # failure without changing filesystem permissions for the rest of the suite.
  diagnostic_capture_stream() {
    local output=$1 metadata=$2
    cat > "$output"
    printf 'bytes=4\ntruncated=0\n' > "$metadata"
    return 7
  }
  lsof() {
    case " $* " in
      *" -iTCP:13000 "*) printf '333\n' ;;
      *) return 1 ;;
    esac
  }
  diagnose_web_ownership_failure capture-failure 13000 >/dev/null
  set -- "$LOG_DIR"/web-ownership.*
  [ "$#" -eq 1 ] || fail "collector failure did not retain one log"
  require_contains "$1" "query.port.listeners.error=capture_failed"
  require_contains "$1" "port.listener.status=unknown"
  require_contains "$1" "port.listeners=unknown"
  require_contains "$1" "port.listener.total=unknown"
  ! grep -Fq 'port.listener.1.pid=' "$1" \
    || fail "collector failure exposed a listener PID"
)

assert_capture_io_failures_propagate() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  local state="$tmp_dir/capture-io" status
  mkdir -p "$state"

  set +e
  printf 'output-data\n' \
    | diagnostic_capture_stream /dev/full "$state/output.meta" 64 >/dev/null 2>&1
  status=$?
  set -e
  [ "$status" -ne 0 ] \
    || fail "collector reported success after output I/O failed"

  set +e
  printf 'metadata-data\n' \
    | diagnostic_capture_stream "$state/output.data" /dev/full 64 >/dev/null 2>&1
  status=$?
  set -e
  [ "$status" -ne 0 ] \
    || fail "collector reported success after metadata I/O failed"
)

assert_diagnostic_budget_cleans_pipe_holder() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  STATE_DIR="$tmp_dir/pipe-holder"
  LOG_DIR="$STATE_DIR/logs"
  mkdir -p "$LOG_DIR"
  # Integer epoch deadlines can lose almost one second at startup. Give the
  # fixture at least one full second to start, while keeping the 3-second
  # assertion below tighter than the legacy 4-second synchronous PGID lookup.
  DIAGNOSTIC_TOTAL_TIMEOUT_SECONDS=2
  DIAGNOSTIC_QUERY_TIMEOUT_SECONDS=2
  holder_script="$STATE_DIR/hold-pipe.sh"
  cat > "$holder_script" <<'EOF'
#!/usr/bin/env bash
sleep 30 &
printf '%s\n' "$!" > "$PIPE_HOLDER_PID_FILE"
exit 0
EOF
  chmod +x "$holder_script"
  PIPE_HOLDER_PID_FILE="$STATE_DIR/holder.pid"
  export PIPE_HOLDER_PID_FILE
  # A legacy synchronous PGID lookup happened before the query deadline was
  # established. If diagnostics regress to calling it, this exceeds the shared
  # budget and the elapsed-time assertion below fails.
  process_group_id() { sleep 4; printf '999'; }
  cleanup_pipe_holder() {
    if [ -s "$PIPE_HOLDER_PID_FILE" ]; then
      holder_pid="$(cat "$PIPE_HOLDER_PID_FILE")"
      kill "$holder_pid" 2>/dev/null || true
    fi
  }
  trap cleanup_pipe_holder EXIT
  lsof() {
    case " $* " in
      *" -iTCP:13000 "*)
        "$holder_script"
        printf '999\n888\n'
        ;;
      *) return 1 ;;
    esac
  }
  started_epoch="$(date +%s)"
  diagnose_web_ownership_failure pipe-holder 13000 >/dev/null
  elapsed=$(( $(date +%s) - started_epoch ))
  [ "$elapsed" -le 3 ] \
    || fail "pipe-holder diagnostic exceeded shared budget: ${elapsed}s"
  [ -s "$PIPE_HOLDER_PID_FILE" ] || fail "pipe-holder fixture did not start"
  holder_pid="$(cat "$PIPE_HOLDER_PID_FILE")"
  if kill -0 "$holder_pid" 2>/dev/null; then
    kill -KILL "$holder_pid" 2>/dev/null || true
    fail "pipe-holder descendant survived diagnostic cleanup"
  fi
  jobs -pr | grep -q . && fail "diagnostic left a background collector/query job"
  set -- "$LOG_DIR"/web-ownership.*
  [ "$#" -eq 1 ] || fail "pipe-holder diagnostic did not retain one log"
  require_contains "$1" "port.listener.total=unknown"
  require_contains "$1" "query.port.listeners.error=capture_timeout"
)

assert_real_nested_ppid_chain() (
  # shellcheck disable=SC1090
  source "$root_dir/scripts/dev-env.sh"
  state="$tmp_dir/real-ppid"
  mkdir -p "$state"
  cleanup_real_nested() {
    for pid_file in "$state/listener" "$state/middle"; do
      if [ -s "$pid_file" ]; then
        pid="$(cat "$pid_file")"
        kill "$pid" 2>/dev/null || true
      fi
    done
    if [ -n "${launcher:-}" ]; then
      kill "$launcher" 2>/dev/null || true
    fi
  }
  trap cleanup_real_nested EXIT
  (
    set -m
    (
      sleep 30 &
      printf '%s\n' "$!" > "$state/listener"
      wait "$!"
    ) &
    printf '%s\n' "$!" > "$state/middle"
    wait "$!"
  ) &
  launcher=$!
  i=0
  while [ ! -s "$state/listener" ] && [ "$i" -lt 20 ]; do
    sleep 0.05
    i=$((i + 1))
  done
  [ -s "$state/listener" ] || fail "real nested PPID fixture did not start"
  listener="$(cat "$state/listener")"
  middle="$(cat "$state/middle")"
  [ "$(process_parent_id "$listener")" = "$middle" ] \
    || fail "listener did not have the real intermediate PPID"
  [ "$(process_parent_id "$middle")" = "$launcher" ] \
    || fail "intermediate process did not have the real launcher PPID"
  [ "$(process_group_id "$listener")" != "$(process_group_id "$launcher")" ] \
    || fail "real nested fixture did not use a distinct process group"
  process_is_descendant_of "$listener" "$launcher" \
    || fail "real nested listener was not found below its launcher"
  kill "$listener" "$middle" "$launcher" 2>/dev/null || true
  wait "$launcher" 2>/dev/null || true
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
assert_pid_parser_boundaries
assert_real_nested_ppid_chain
assert_signal_allowlists_default_deny

# The added diagnostics are exercised through real start_web, ownership,
# registration, and stop decisions. Only query/signal primitives are mocked.
assert_web_start_case legal-nested 0 420 'listener.recorded_pid=unknown'
assert_web_start_case recorded-pid 0 420 'listener.recorded_pid=420'
assert_web_start_case foreign-listener 1 "" 'listener.replaced=0'
assert_web_start_case foreign-negative-pgid 1 "" 'listener.replaced=0'
assert_web_start_case parent-disconnected 1 "" 'parent_chain.listener=999->1'
assert_web_start_case parent-timeout 1 "" 'parent_chain.listener=999->?'
assert_web_start_case parent-failure 1 "" 'parent_chain.listener=999->?'
assert_web_start_case pgid-budget 1 "" 'process.listener.pid=999'
assert_web_start_case launcher-exited 1 "" 'process.launcher.live=0'
assert_web_start_case listener-replaced 1 "" 'listener.replaced=1'
assert_web_start_case multi-listener 1 "" 'port.listener.2.pid=888'
assert_web_start_case long-output 1 "" 'port.listener.count=1'
assert_web_start_case query-failure 1 "" 'query.port.listeners.error=command_failed'
assert_web_start_case timeout 1 "" 'query.port.listeners.error=timeout'
assert_web_start_case no-listener 1 "" 'port.listeners=none'
assert_web_start_case foreign-positive 1 "" 'port.listener.1.pid=777'
assert_web_start_case foreign-negative 1 "" 'port.listener.status=unknown'
assert_web_start_case log-create-failure 1 "" ''
assert_web_start_case log-write-failure 1 "" ''
assert_diagnostic_logs_are_unique
assert_diagnostic_enum_resets_between_calls
assert_capture_failure_reports_unknown
assert_capture_io_failures_propagate
assert_diagnostic_budget_cleans_pipe_holder

# Stopping first records an owned nested listener before killing the launcher's
# process group. An unrelated port occupant never receives a signal.
assert_stop_handles_listener nested 420 310 200 "" 420
assert_stop_handles_listener external 999 999 1 888
assert_stop_handles_listener external-positive 777 777 1 888
assert_stop_handles_listener external-negative 777 777 1 -777
assert_stop_restores_same_pgid_listener

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
