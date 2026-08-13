#!/bin/sh
set -eu

SERVER="https://supportgenesis.ru"
TOKEN=""
NAME=""

usage() {
  printf '%s\n' "Usage: install-remoteit.sh --token TOKEN [--name NAME] [--server HTTPS_URL]"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --token) TOKEN="${2-}"; shift 2 ;;
    --name) NAME="${2-}"; shift 2 ;;
    --server) SERVER="${2-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$SERVER" in
  https://*) ;;
  *) printf '%s\n' "Only an HTTPS RemoteIt server is allowed." >&2; exit 2 ;;
esac
[ -n "$TOKEN" ] || { printf '%s\n' "The enrollment token is required." >&2; exit 2; }
default_name="$(hostname 2>/dev/null || printf 'RemoteIt device')"

valid_computer_name() {
  candidate="$1"
  [ -n "$candidate" ] || return 1
  [ "${#candidate}" -le 64 ] || return 1
  case "$candidate" in
    *"\n"*|*"\r"*|*"--token"*|*"https://"*|*"http://"*|*"command -v"*|curl\ *|wget\ *|sudo\ *|sh\ *|bash\ *|if\ *|then\ *|fi\ *) return 1 ;;
  esac
  return 0
}

if [ -z "$NAME" ]; then
  if [ -t 0 ]; then
    attempts=0
    while :; do
      printf 'Computer name [%s]: ' "$default_name"
      IFS= read -r NAME || NAME=""
      NAME="$(printf '%s' "$NAME" | tr -d '\r\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
      [ -n "$NAME" ] || NAME="$default_name"
      if valid_computer_name "$NAME"; then
        break
      fi
      attempts=$((attempts + 1))
      printf '%s\n' 'RemoteIt Agent: в поле имени попала команда или имя длиннее 64 символов.' >&2
      if [ "$attempts" -ge 2 ]; then
        NAME="$default_name"
        printf 'RemoteIt Agent: используется имя по умолчанию: %s\n' "$NAME" >&2
        break
      fi
      NAME=""
    done
  else
    NAME="$default_name"
  fi
else
  NAME="$(printf '%s' "$NAME" | tr -d '\r\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  if ! valid_computer_name "$NAME"; then
    printf '%s\n' 'RemoteIt Agent: имя компьютера должно содержать 1–64 символа и не может быть командой.' >&2
    exit 2
  fi
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os:$arch" in
  linux:x86_64|linux:amd64) artifact="remoteit-agent-linux-amd64" ;;
  darwin:x86_64|darwin:amd64) artifact="remoteit-agent-macos-amd64" ;;
  darwin:arm64|darwin:aarch64) artifact="remoteit-agent-macos-arm64" ;;
  *) printf 'Unsupported platform: %s/%s\n' "$os" "$arch" >&2; exit 3 ;;
esac

temporary="$(mktemp "${TMPDIR:-/tmp}/remoteit-agent.XXXXXX")"
checksums="$(mktemp "${TMPDIR:-/tmp}/remoteit-checksums.XXXXXX")"
trap 'rm -f "$temporary" "$checksums"' EXIT HUP INT TERM

base="${SERVER%/}/downloads"
download() {
  source_url="$1"
  destination="$2"
  if command -v curl >/dev/null 2>&1; then
    curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$source_url" --output "$destination"
  elif command -v wget >/dev/null 2>&1; then
    wget --quiet --https-only --output-document="$destination" "$source_url"
  else
    printf '%s\n' "RemoteIt needs curl or wget to download the Agent." >&2
    exit 5
  fi
}

# Minimal Ubuntu images often have neither curl nor wget. A copied installation
# command must therefore remain self-contained: install one downloader through
# the native package manager when possible instead of failing before RemoteIt
# can even verify its signed checksum manifest. Package-manager output stays
# visible because sudo may legitimately ask the local user for a password.
ensure_downloader() {
  if command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1; then
    return 0
  fi
  run_privileged() {
    if [ "$(id -u)" -eq 0 ]; then
      "$@"
    elif command -v sudo >/dev/null 2>&1; then
      sudo "$@"
    else
      printf '%s\n' "RemoteIt needs root privileges to install curl." >&2
      return 1
    fi
  }
  if command -v apt-get >/dev/null 2>&1; then
    run_privileged apt-get update && run_privileged apt-get install -y --no-install-recommends ca-certificates curl
  elif command -v apk >/dev/null 2>&1; then
    run_privileged apk add --no-cache ca-certificates curl
  elif command -v dnf >/dev/null 2>&1; then
    run_privileged dnf install -y ca-certificates curl
  elif command -v yum >/dev/null 2>&1; then
    run_privileged yum install -y ca-certificates curl
  elif command -v zypper >/dev/null 2>&1; then
    run_privileged zypper --non-interactive install ca-certificates curl
  else
    printf '%s\n' "RemoteIt needs curl or wget and could not find a supported package manager." >&2
    return 1
  fi
}

ensure_downloader || exit 5

download "$base/$artifact" "$temporary"
download "$base/SHA256SUMS.txt" "$checksums"

expected="$(awk -v name="$artifact" '$2 == name { print $1 }' "$checksums")"
[ -n "$expected" ] || { printf '%s\n' "The checksum manifest does not contain $artifact." >&2; exit 4; }
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$temporary" | awk '{print $1}')"
fi
[ "$actual" = "$expected" ] || { printf '%s\n' "RemoteIt Agent checksum mismatch." >&2; exit 4; }

chmod 700 "$temporary"
if [ "$(id -u)" -eq 0 ]; then
  "$temporary" install --token "$TOKEN" --name "$NAME" --server "$SERVER"
else
  sudo "$temporary" install --token "$TOKEN" --name "$NAME" --server "$SERVER"
fi
printf '%s\n' "RemoteIt Agent installed and enrolled successfully."
