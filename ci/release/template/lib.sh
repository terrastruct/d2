#!/bin/sh
set -eu

_echo() {
  printf '%s\n' "$*"
}

tput() {
  if [ -n "$TERM" ]; then
    command tput "$@"
  fi
}

setaf() {
  tput setaf "$1"
  shift
  printf '%s' "$*"
  tput sgr0
}

echoerr() {
  COLOR=1 echop err "$*" >&2
}

printferr() {
  COLOR=1 printfp err "$@" >&2
}

sh_c() {
  COLOR=3 echop exec "$*"
  "$@"
}

get_rand_color() {
  # 1-6 are regular and 9-14 are bright.
  # 1,2 and 9,10 are red and green but we use those for success and failure.
  pick "$*" 3 4 5 6 11 12 13 14
}

echop() {(
  prefix="$1"
  shift

  if [ -z "${COLOR:-}" ]; then
    COLOR="$(get_rand_color "$prefix")"
  fi
  printf '%s: %s\n' "$(setaf "$COLOR" "$prefix")" "$*"
)}

printfp() {(
  PREFIX="$1"
  shift
  echop "$PREFIX" "$(printf "$@")"
)}

hide() {
  out="$(mktemp)"
  set +e
  "$@" >"$out" 2>&1
  code="$?"
  set -e
  if [ "$code" -eq 0 ]; then
    return
  fi
  cat "$out" >&2
  exit "$code"
}

echo_dur() {
  local dur=$1
  local h=$((dur/60/60))
  local m=$((dur/60%60))
  local s=$((dur%60))
  printf '%dh%dm%ds' "$h" "$m" "$s"
}

sponge() {
  dst="$1"
  tmp="$(mktemp)"
  cat > "$tmp"
  cat "$tmp" > "$dst"
}

stripansi() {
  # First regex gets rid of standard xterm escape sequences for controlling
  # visual attributes.
  # The second regex I'm not 100% sure, the reference says it selects the US
  # encoding but I'm not sure why that's necessary or why it always occurs
  # in tput sgr0 before the standard escape sequence.
  # See tput sgr0 | xxd
  sed -e $'s/\x1b\[[0-9;]*m//g' -e $'s/\x1b(.//g'
}

runtty() {
  case "$(uname)" in
    Darwin)
      script -q /dev/null "$@"
      ;;
    Linux)
      script -eqc "$*"
      ;;
    *)
      echoerr "runtty: unsupported OS $(uname)"
  esac
}
