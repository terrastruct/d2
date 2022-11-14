#!/bin/sh

rand() {
  seed="$1"
  range="$2"

  seed_file="$(mktemp)"
  _echo "$seed" | md5sum > "$seed_file"
  shuf -i "$range" -n 1 --random-source="$seed_file"
}

pick() {
  seed="$1"
  shift
  i="$(rand "$seed" "1-$#")"
  eval "_echo \"\$$i\""
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

_echo() {
  printf '%s\n' "$*"
}

get_rand_color() {
  # 1-6 are regular and 9-14 are bright.
  # 1,2 and 9,10 are red and green but we use those for success and failure.
  pick "$*" 3 4 5 6 11 12 13 14
}

echop() {
  prefix="$1"
  shift

  if [ "$#" -gt 0 ]; then
    printfp "$prefix" "%s\n" "$*"
  else
    printfp "$prefix"
    printf '\n'
  fi
}

printfp() {(
  prefix="$1"
  shift

  if [ -z "${COLOR:-}" ]; then
    COLOR="$(get_rand_color "$prefix")"
  fi
  printf '%s' "$(setaf "$COLOR" "$prefix")"

  if [ $# -gt 0 ]; then
    printf ': '
    printf "$@"
  fi
)}

catp() {
  prefix="$1"
  shift

  printfp "$prefix"
  printf ': '
  read -r line
  _echo "$line"

  indent=$(repeat ' ' 2)
  sed "s/^/$indent/"
}

repeat() {
  char="$1"
  times="$2"
  seq -s "$char" "$times" | tr -d '[:digit:]'
}

strlen() {
  printf %s "$1" | wc -c
}

echoerr() {
  COLOR=1 echop err "$*" >&2
}

caterr() {
  COLOR=1 catp err "$@" >&2
}

printferr() {
  COLOR=1 printfp err "$@" >&2
}

logp() {
  echop "$@" >&2
}

logfp() {
  printfp "$@" >&2
}

logpcat() {
  catp "$@" >&2
}

log() {
  COLOR=5 logp log "$@"
}

logf() {
  COLOR=5 logfp log "$@"
}

logcat() {
  COLOR=5 catp log "$@" >&2
}

sh_c() {
  COLOR=3 logp exec "$*"
  if [ -z "${DRY_RUN-}" ]; then
    "$@"
  fi
}

header() {
  logp "/* $1 */"
}
