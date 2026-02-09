#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env"
ENV_EXAMPLE="$SCRIPT_DIR/.env.example"
BIN_DIR="$SCRIPT_DIR/bin"
BIN_PATH="$BIN_DIR/hosibot"
RUNTIME_DIR="$SCRIPT_DIR/runtime"
LOG_FILE="$RUNTIME_DIR/hosibot.log"
PID_FILE="$RUNTIME_DIR/hosibot.pid"
SERVICE_NAME="${SERVICE_NAME:-hosibot.service}"
RELEASE_REPO="${RELEASE_REPO:-FakharzadehH/hosibot}"
RELEASE_API_URL="${RELEASE_API_URL:-https://api.github.com/repos/${RELEASE_REPO}/releases/latest}"

# Colors
C_RESET='\033[0m'
C_BOLD='\033[1m'
C_BLUE='\033[34m'
C_CYAN='\033[36m'
C_GREEN='\033[32m'
C_YELLOW='\033[33m'
C_RED='\033[31m'
C_GRAY='\033[90m'

line() {
  printf "%b\n" "${C_GRAY}------------------------------------------------------------${C_RESET}"
}

banner() {
  clear || true
  printf "%b\n" "${C_BOLD}${C_BLUE}"
  cat <<'BANNER'
 _   _           _ _           _
| | | | ___  ___(_) |__   ___ | |_ 
| |_| |/ _ \/ __| | '_ \ / _ \| __|
|  _  | (_) \__ \ | |_) | (_) | |_ 
|_| |_|\___/|___/_|_.__/ \___/ \__|
BANNER
  printf "%b\n" "${C_RESET}${C_CYAN}Go Deployment Manager${C_RESET}"
  line
}

info() { printf "%b\n" "${C_BLUE}[INFO]${C_RESET} $*"; }
success() { printf "%b\n" "${C_GREEN}[OK]${C_RESET} $*"; }
warn() { printf "%b\n" "${C_YELLOW}[WARN]${C_RESET} $*"; }
error() { printf "%b\n" "${C_RED}[ERR]${C_RESET} $*"; }

pause() {
  if [[ -t 0 ]]; then
    read -r -p "Press Enter to continue..." _
  fi
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    error "Missing command: $cmd"
    return 1
  fi
}

run_root() {
  if [[ $EUID -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    error "This action needs root privileges and sudo is not available."
    return 1
  fi
}

ensure_dirs() {
  mkdir -p "$BIN_DIR" "$RUNTIME_DIR"
}

ensure_env_file() {
  if [[ ! -f "$ENV_FILE" ]]; then
    if [[ -f "$ENV_EXAMPLE" ]]; then
      cp "$ENV_EXAMPLE" "$ENV_FILE"
      success "Created $ENV_FILE from .env.example"
    else
      touch "$ENV_FILE"
      warn "Created empty $ENV_FILE (example file not found)"
    fi
  fi
}

load_env() {
  ensure_env_file
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
}

escape_sed() {
  printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'
}

set_env_value() {
  local key="$1"
  local value="$2"
  ensure_env_file
  if grep -q "^${key}=" "$ENV_FILE"; then
    sed -i "s|^${key}=.*$|${key}=$(escape_sed "$value")|" "$ENV_FILE"
  else
    printf "%s=%s\n" "$key" "$value" >>"$ENV_FILE"
  fi
}

get_env_value() {
  local key="$1"
  if [[ -f "$ENV_FILE" ]]; then
    grep -E "^${key}=" "$ENV_FILE" | tail -n1 | cut -d'=' -f2-
  fi
}

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
  else
    tr -dc 'A-Za-z0-9' </dev/urandom | head -c 48
  fi
}

mask_value() {
  local v="$1"
  local n=${#v}
  if (( n <= 6 )); then
    printf "******"
  else
    printf "%s***%s" "${v:0:3}" "${v:n-3:3}"
  fi
}

normalize_domain() {
  local d="${1:-}"
  d="${d%/}"
  if [[ -n "$d" && ! "$d" =~ ^https?:// ]]; then
    d="https://$d"
  fi
  printf "%s" "$d"
}

escape_sql_string() {
  printf "%s" "$1" | sed "s/'/''/g"
}

escape_sql_ident() {
  printf "%s" "$1" | sed 's/`/``/g'
}

build_default_webhook() {
  local domain="$1"
  local token="$2"
  if [[ -z "$domain" || -z "$token" ]]; then
    return 0
  fi
  printf "%s/webhook/%s" "${domain%/}" "$token"
}

confirm() {
  local prompt="$1"
  local default="${2:-N}"
  local suffix="[y/N]"
  if [[ "$default" == "Y" ]]; then
    suffix="[Y/n]"
  fi

  read -r -p "$prompt $suffix: " ans
  ans="${ans:-$default}"
  [[ "$ans" =~ ^[Yy]$ ]]
}

print_runtime_status() {
  if [[ -f "$PID_FILE" ]]; then
    local pid
    pid="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      printf "%b\n" "Background process: ${C_GREEN}RUNNING${C_RESET} (pid $pid)"
      return
    fi
  fi
  printf "%b\n" "Background process: ${C_RED}STOPPED${C_RESET}"
}

print_service_status() {
  if command -v systemctl >/dev/null 2>&1; then
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
      printf "%b\n" "Systemd service:   ${C_GREEN}ACTIVE${C_RESET} ($SERVICE_NAME)"
    else
      printf "%b\n" "Systemd service:   ${C_RED}INACTIVE${C_RESET} ($SERVICE_NAME)"
    fi
  else
    printf "%b\n" "Systemd service:   ${C_YELLOW}systemctl not found${C_RESET}"
  fi
}

install_mysql_stack() {
  info "Installing MySQL-compatible database packages (server + client)"

  if command -v apt-get >/dev/null 2>&1; then
    if run_root apt-get install -y default-mysql-server default-mysql-client; then
      success "Installed default-mysql packages"
    elif run_root apt-get install -y mariadb-server mariadb-client; then
      success "Installed MariaDB packages"
    elif run_root apt-get install -y mysql-server mysql-client; then
      success "Installed MySQL packages"
    else
      warn "Could not install MySQL/MariaDB packages via apt"
    fi
    return
  fi

  if command -v dnf >/dev/null 2>&1; then
    if run_root dnf install -y mariadb-server mariadb; then
      success "Installed MariaDB packages"
    elif run_root dnf install -y mysql-server mysql; then
      success "Installed MySQL packages"
    else
      warn "Could not install MySQL/MariaDB packages via dnf"
    fi
    return
  fi

  if command -v yum >/dev/null 2>&1; then
    if run_root yum install -y mariadb-server mariadb; then
      success "Installed MariaDB packages"
    elif run_root yum install -y mysql-server mysql; then
      success "Installed MySQL packages"
    else
      warn "Could not install MySQL/MariaDB packages via yum"
    fi
    return
  fi

  if command -v pacman >/dev/null 2>&1; then
    if run_root pacman -Sy --noconfirm mariadb; then
      success "Installed MariaDB package"
    else
      warn "Could not install MariaDB package via pacman"
    fi
    return
  fi

  warn "Unsupported package manager. Install MySQL/MariaDB manually."
}

install_redis_stack() {
  info "Installing Redis packages (server + cli)"

  if command -v apt-get >/dev/null 2>&1; then
    if run_root apt-get install -y redis-server redis-tools; then
      success "Installed Redis packages"
    else
      warn "Could not install Redis packages via apt"
    fi
    return
  fi

  if command -v dnf >/dev/null 2>&1; then
    if run_root dnf install -y redis; then
      success "Installed Redis package"
    else
      warn "Could not install Redis package via dnf"
    fi
    return
  fi

  if command -v yum >/dev/null 2>&1; then
    if run_root yum install -y redis; then
      success "Installed Redis package"
    else
      warn "Could not install Redis package via yum"
    fi
    return
  fi

  if command -v pacman >/dev/null 2>&1; then
    if run_root pacman -Sy --noconfirm redis; then
      success "Installed Redis package"
    else
      warn "Could not install Redis package via pacman"
    fi
    return
  fi

  warn "Unsupported package manager. Install Redis manually."
}

start_mysql_service() {
  if ! command -v systemctl >/dev/null 2>&1; then
    warn "systemctl not found; skipping automatic DB service start"
    return
  fi

  local svc
  for svc in mysql mariadb mysqld; do
    if systemctl list-unit-files | awk '{print $1}' | grep -qx "${svc}.service"; then
      run_root systemctl enable --now "${svc}.service" || true
      if systemctl is-active --quiet "${svc}.service"; then
        success "Database service is active: ${svc}.service"
      else
        warn "Database service exists but is not active: ${svc}.service"
      fi
      return
    fi
  done

  warn "No mysql/mariadb systemd service unit detected"
}

start_redis_service() {
  if ! command -v systemctl >/dev/null 2>&1; then
    warn "systemctl not found; skipping Redis service start"
    return
  fi

  local svc
  for svc in redis redis-server; do
    if systemctl list-unit-files | awk '{print $1}' | grep -qx "${svc}.service"; then
      run_root systemctl enable --now "${svc}.service" || true
      if systemctl is-active --quiet "${svc}.service"; then
        success "Redis service is active: ${svc}.service"
      else
        warn "Redis service exists but is not active: ${svc}.service"
      fi
      return
    fi
  done

  warn "No redis systemd service unit detected"
}

bootstrap_database() {
  banner
  load_env

  local mysql_bin=""
  if command -v mysql >/dev/null 2>&1; then
    mysql_bin="mysql"
  elif command -v mariadb >/dev/null 2>&1; then
    mysql_bin="mariadb"
  fi

  if [[ -z "$mysql_bin" ]]; then
    error "Neither mysql nor mariadb client command is installed."
    pause
    return
  fi

  local db_host db_port db_name db_user db_pass db_charset
  db_host="${DB_HOST:-localhost}"
  db_port="${DB_PORT:-3306}"
  db_name="${DB_NAME:-}"
  db_user="${DB_USER:-}"
  db_pass="${DB_PASS:-}"
  db_charset="${DB_CHARSET:-utf8mb4}"

  if [[ -z "$db_name" || -z "$db_user" ]]; then
    error "DB_NAME and DB_USER must be set in $ENV_FILE before bootstrapping."
    pause
    return
  fi

  local db_name_esc db_user_esc db_pass_esc db_charset_esc
  db_name_esc="$(escape_sql_ident "$db_name")"
  db_user_esc="$(escape_sql_string "$db_user")"
  db_pass_esc="$(escape_sql_string "$db_pass")"
  db_charset_esc="$(escape_sql_ident "$db_charset")"

  local create_db_sql
  create_db_sql="CREATE DATABASE IF NOT EXISTS \`$db_name_esc\` CHARACTER SET $db_charset_esc;"

  info "Bootstrapping database using current .env credentials"
  info "Target DB: $db_name | User: $db_user | Host: $db_host:$db_port"

  if [[ -n "$db_pass" ]]; then
    if MYSQL_PWD="$db_pass" "$mysql_bin" -h "$db_host" -P "$db_port" -u "$db_user" -e "$create_db_sql" >/dev/null 2>&1; then
      success "Database '$db_name' ensured via app credentials."
    else
      warn "App credentials could not create database directly. Trying local admin bootstrap."
    fi
  else
    if "$mysql_bin" -h "$db_host" -P "$db_port" -u "$db_user" -e "$create_db_sql" >/dev/null 2>&1; then
      success "Database '$db_name' ensured via app credentials."
    else
      warn "App credentials could not create database directly. Trying local admin bootstrap."
    fi
  fi

  local bootstrap_ok=0
  if [[ "$db_user" == "root" ]]; then
    if run_root "$mysql_bin" --protocol=socket -e "$create_db_sql" >/dev/null 2>&1; then
      bootstrap_ok=1
      success "Database '$db_name' created/verified with local root admin."
    fi
  else
    local admin_sql
    admin_sql="$create_db_sql"$'\n'
    admin_sql+="CREATE USER IF NOT EXISTS '$db_user_esc'@'localhost' IDENTIFIED BY '$db_pass_esc';"$'\n'
    admin_sql+="CREATE USER IF NOT EXISTS '$db_user_esc'@'%' IDENTIFIED BY '$db_pass_esc';"$'\n'
    admin_sql+="ALTER USER '$db_user_esc'@'localhost' IDENTIFIED BY '$db_pass_esc';"$'\n'
    admin_sql+="ALTER USER '$db_user_esc'@'%' IDENTIFIED BY '$db_pass_esc';"$'\n'
    admin_sql+="GRANT ALL PRIVILEGES ON \`$db_name_esc\`.* TO '$db_user_esc'@'localhost';"$'\n'
    admin_sql+="GRANT ALL PRIVILEGES ON \`$db_name_esc\`.* TO '$db_user_esc'@'%';"$'\n'
    admin_sql+="FLUSH PRIVILEGES;"

    if run_root "$mysql_bin" --protocol=socket -e "$admin_sql" >/dev/null 2>&1; then
      bootstrap_ok=1
      success "Database and grants bootstrapped for '$db_user'."
    fi
  fi

  if [[ "$bootstrap_ok" -eq 0 ]]; then
    warn "Local admin bootstrap failed or was unavailable. Continuing with credential-based verification."
  fi

  if [[ -n "$db_pass" ]]; then
    if MYSQL_PWD="$db_pass" "$mysql_bin" -h "$db_host" -P "$db_port" -u "$db_user" -D "$db_name" -e "SELECT 1;" >/dev/null 2>&1; then
      success "Database connection test passed with DB_USER/DB_PASS."
    else
      error "Database connection test failed for DB_USER/DB_PASS."
      warn "Check DB_HOST/DB_PORT/DB_NAME/DB_USER/DB_PASS in $ENV_FILE and MySQL auth settings."
    fi
  else
    if "$mysql_bin" -h "$db_host" -P "$db_port" -u "$db_user" -D "$db_name" -e "SELECT 1;" >/dev/null 2>&1; then
      success "Database connection test passed with DB_USER (no password)."
    else
      error "Database connection test failed for DB_USER with empty password."
      warn "Set DB_PASS in $ENV_FILE if your MySQL user requires a password."
    fi
  fi

  pause
}

install_dependencies() {
  banner
  info "Installing deployment dependencies"

  local pkgs=(curl git ca-certificates jq tar gzip unzip python3)

  if command -v apt-get >/dev/null 2>&1; then
    run_root apt-get update
    run_root apt-get install -y "${pkgs[@]}"
  elif command -v dnf >/dev/null 2>&1; then
    run_root dnf install -y "${pkgs[@]}"
  elif command -v yum >/dev/null 2>&1; then
    run_root yum install -y "${pkgs[@]}"
  elif command -v pacman >/dev/null 2>&1; then
    run_root pacman -Sy --noconfirm "${pkgs[@]}"
  else
    warn "No supported package manager found. Install curl/git/jq/python3 manually."
  fi

  if command -v jq >/dev/null 2>&1; then
    success "jq found: $(jq --version)"
  else
    warn "jq is missing (script will fallback to python3/json parsing)."
  fi

  install_mysql_stack
  start_mysql_service

  if command -v mysql >/dev/null 2>&1; then
    success "mysql client found: $(mysql --version | head -n1)"
  else
    warn "mysql client still not found. Install DB client manually if required."
  fi

  install_redis_stack
  start_redis_service

  if command -v redis-cli >/dev/null 2>&1; then
    success "redis-cli found: $(redis-cli --version)"
  else
    warn "redis-cli still not found. Install Redis client manually if required."
  fi

  pause
}

configure_env_wizard() {
  banner
  ensure_env_file
  load_env

  info "Environment wizard"
  line

  local app_port app_env db_host db_port db_name db_user db_pass db_charset
  local redis_addr redis_pass redis_db
  local bot_token bot_domain bot_webhook bot_admin bot_username api_key jwt_secret

  app_port="${APP_PORT:-8080}"
  app_env="${APP_ENV:-production}"
  db_host="${DB_HOST:-localhost}"
  db_port="${DB_PORT:-3306}"
  db_name="${DB_NAME:-hosibot}"
  db_user="${DB_USER:-root}"
  db_pass="${DB_PASS:-}"
  db_charset="${DB_CHARSET:-utf8mb4}"
  redis_addr="${REDIS_ADDR:-localhost:6379}"
  redis_pass="${REDIS_PASS:-}"
  redis_db="${REDIS_DB:-0}"
  bot_token="${BOT_TOKEN:-}"
  bot_domain="$(normalize_domain "${BOT_DOMAIN:-}")"
  bot_admin="${BOT_ADMIN_ID:-}"
  bot_username="${BOT_USERNAME:-}"
  api_key="${API_KEY:-$(random_secret)}"
  jwt_secret="${JWT_SECRET:-$(random_secret)}"

  read -r -p "APP_PORT [$app_port]: " input; app_port="${input:-$app_port}"
  read -r -p "APP_ENV [$app_env]: " input; app_env="${input:-$app_env}"

  read -r -p "DB_HOST [$db_host]: " input; db_host="${input:-$db_host}"
  read -r -p "DB_PORT [$db_port]: " input; db_port="${input:-$db_port}"
  read -r -p "DB_NAME [$db_name]: " input; db_name="${input:-$db_name}"
  read -r -p "DB_USER [$db_user]: " input; db_user="${input:-$db_user}"
  read -r -s -p "DB_PASS [hidden, Enter to keep current]: " input; printf "\n"
  if [[ -n "$input" ]]; then db_pass="$input"; fi
  read -r -p "DB_CHARSET [$db_charset]: " input; db_charset="${input:-$db_charset}"
  read -r -p "REDIS_ADDR [$redis_addr]: " input; redis_addr="${input:-$redis_addr}"
  read -r -s -p "REDIS_PASS [hidden, Enter to keep current]: " input; printf "\n"
  if [[ -n "$input" ]]; then redis_pass="$input"; fi
  read -r -p "REDIS_DB [$redis_db]: " input; redis_db="${input:-$redis_db}"

  read -r -p "BOT_TOKEN [required, current: $(mask_value "$bot_token")]: " input
  if [[ -n "$input" ]]; then bot_token="$input"; fi

  read -r -p "BOT_DOMAIN [$bot_domain] (example: https://example.com): " input
  if [[ -n "$input" ]]; then bot_domain="$(normalize_domain "$input")"; fi

  local suggested_webhook
  suggested_webhook="$(build_default_webhook "$bot_domain" "$bot_token")"
  bot_webhook="${BOT_WEBHOOK_URL:-$suggested_webhook}"
  read -r -p "BOT_WEBHOOK_URL [$bot_webhook]: " input
  bot_webhook="${input:-$bot_webhook}"

  read -r -p "BOT_ADMIN_ID [$bot_admin]: " input; bot_admin="${input:-$bot_admin}"
  read -r -p "BOT_USERNAME [$bot_username]: " input; bot_username="${input:-$bot_username}"

  read -r -p "API_KEY [auto-generated, current masked: $(mask_value "$api_key")]: " input
  if [[ -n "$input" ]]; then api_key="$input"; fi

  read -r -p "JWT_SECRET [auto-generated, current masked: $(mask_value "$jwt_secret")]: " input
  if [[ -n "$input" ]]; then jwt_secret="$input"; fi

  if [[ -z "$bot_token" ]]; then
    error "BOT_TOKEN cannot be empty."
    pause
    return
  fi

  set_env_value "APP_PORT" "$app_port"
  set_env_value "APP_ENV" "$app_env"
  set_env_value "DB_HOST" "$db_host"
  set_env_value "DB_PORT" "$db_port"
  set_env_value "DB_NAME" "$db_name"
  set_env_value "DB_USER" "$db_user"
  set_env_value "DB_PASS" "$db_pass"
  set_env_value "DB_CHARSET" "$db_charset"
  set_env_value "REDIS_ADDR" "$redis_addr"
  set_env_value "REDIS_PASS" "$redis_pass"
  set_env_value "REDIS_DB" "$redis_db"

  set_env_value "BOT_TOKEN" "$bot_token"
  set_env_value "BOT_DOMAIN" "$bot_domain"
  set_env_value "BOT_WEBHOOK_URL" "$bot_webhook"
  set_env_value "BOT_ADMIN_ID" "$bot_admin"
  set_env_value "BOT_USERNAME" "$bot_username"

  set_env_value "API_KEY" "$api_key"
  set_env_value "JWT_SECRET" "$jwt_secret"

  success "Saved configuration to $ENV_FILE"
  pause
}

normalize_os_arch() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m | tr '[:upper:]' '[:lower:]')"

  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    armv7l|armv7) arch="armv7" ;;
    armv6l|armv6) arch="armv6" ;;
    i386|i686) arch="386" ;;
    *) ;;
  esac

  printf "%s %s" "$os" "$arch"
}

resolve_latest_release_asset() {
  local json_file="$1"
  local os="$2"
  local arch="$3"

  if command -v jq >/dev/null 2>&1; then
    jq -r --arg os "$os" --arg arch "$arch" '
      def pick_asset:
        (
          [.assets[] | {
            name: (.name // ""),
            lname: ((.name // "") | ascii_downcase),
            url: (.browser_download_url // ""),
            digest: (.digest // "")
          }] as $a
          | (
              ($a | map(select((.lname | test($os)) and (.lname | test($arch)))) | .[0])
              // ($a | map(select((.url | ascii_downcase | test($os)) and (.url | ascii_downcase | test($arch)))) | .[0])
              // ($a | map(select(.name == "hosibot")) | .[0])
              // ($a | .[0])
            )
        );
      (pick_asset) as $picked
      | [.tag_name, .html_url, ($picked.name // ""), ($picked.url // ""), ($picked.digest // "")]
      | @tsv
    ' "$json_file"
    return
  fi

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$json_file" "$os" "$arch" <<'PY'
import json, sys
path, os_name, arch = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, "r", encoding="utf-8") as f:
    data = json.load(f)
assets = data.get("assets", []) or []
picked = None
for a in assets:
    n = (a.get("name") or "").lower()
    u = (a.get("browser_download_url") or "").lower()
    if os_name in n and arch in n:
        picked = a
        break
if not picked:
    for a in assets:
        u = (a.get("browser_download_url") or "").lower()
        if os_name in u and arch in u:
            picked = a
            break
if not picked:
    for a in assets:
        if (a.get("name") or "") == "hosibot":
            picked = a
            break
if not picked and assets:
    picked = assets[0]
tag = data.get("tag_name", "")
html = data.get("html_url", "")
name = picked.get("name", "") if picked else ""
url = picked.get("browser_download_url", "") if picked else ""
digest = picked.get("digest", "") if picked else ""
print("\t".join([tag, html, name, url, digest]))
PY
    return
  fi

  local tag release_url asset_url asset_name
  tag="$(grep -m1 '"tag_name"' "$json_file" | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  release_url="$(grep -m1 '"html_url"' "$json_file" | sed -E 's/.*"html_url": *"([^"]+)".*/\1/')"
  asset_url="$(grep -m1 '"browser_download_url"' "$json_file" | sed -E 's/.*"browser_download_url": *"([^"]+)".*/\1/')"
  asset_name="$(basename "$asset_url")"
  printf "%s\t%s\t%s\t%s\t%s\n" "$tag" "$release_url" "$asset_name" "$asset_url" ""
}

build_binary() {
  banner
  require_cmd curl || { pause; return; }
  ensure_dirs

  local os arch
  read -r os arch <<<"$(normalize_os_arch)"
  info "Detected platform: ${os}/${arch}"

  local tmp_dir json_file
  tmp_dir="$(mktemp -d)"
  json_file="$tmp_dir/release.json"

  info "Fetching latest release metadata from $RELEASE_REPO"
  if ! curl -fsSL -H "Accept: application/vnd.github+json" "$RELEASE_API_URL" -o "$json_file"; then
    error "Failed to fetch latest release metadata"
    rm -rf "$tmp_dir"
    pause
    return
  fi

  local tag release_url asset_name asset_url digest
  IFS=$'\t' read -r tag release_url asset_name asset_url digest < <(resolve_latest_release_asset "$json_file" "$os" "$arch")

  if [[ -z "${asset_url:-}" ]]; then
    error "No downloadable asset found in latest release."
    rm -rf "$tmp_dir"
    pause
    return
  fi

  info "Latest release: ${tag:-unknown}"
  info "Release page: ${release_url:-N/A}"
  info "Selected asset: ${asset_name:-$(basename "$asset_url")}"

  local download_path
  download_path="$tmp_dir/${asset_name:-asset.bin}"

  info "Downloading asset..."
  if ! curl -fL --retry 3 --retry-delay 2 "$asset_url" -o "$download_path"; then
    error "Failed to download release asset"
    rm -rf "$tmp_dir"
    pause
    return
  fi

  local extracted_bin=""
  case "$download_path" in
    *.tar.gz|*.tgz)
      tar -xzf "$download_path" -C "$tmp_dir"
      extracted_bin="$(find "$tmp_dir" -maxdepth 3 -type f -name 'hosibot*' -perm -u+x | head -n1 || true)"
      ;;
    *.zip)
      if command -v unzip >/dev/null 2>&1; then
        unzip -o "$download_path" -d "$tmp_dir" >/dev/null
        extracted_bin="$(find "$tmp_dir" -maxdepth 3 -type f -name 'hosibot*' -perm -u+x | head -n1 || true)"
      fi
      ;;
    *)
      extracted_bin="$download_path"
      ;;
  esac

  if [[ -z "$extracted_bin" || ! -f "$extracted_bin" ]]; then
    error "Could not locate hosibot binary inside downloaded asset"
    rm -rf "$tmp_dir"
    pause
    return
  fi

  if [[ -n "$digest" && "$digest" == sha256:* ]] && command -v sha256sum >/dev/null 2>&1; then
    local expected actual
    expected="${digest#sha256:}"
    actual="$(sha256sum "$extracted_bin" | awk '{print $1}')"
    if [[ "$expected" == "$actual" ]]; then
      success "SHA256 checksum verified"
    else
      warn "Checksum mismatch (expected $expected got $actual)"
    fi
  fi

  install -m 755 "$extracted_bin" "$BIN_PATH"
  success "Binary downloaded and installed: $BIN_PATH"
  rm -rf "$tmp_dir"
  pause
}

run_foreground() {
  banner
  if [[ ! -x "$BIN_PATH" ]]; then
    warn "Binary not found. Downloading latest release first."
    build_binary
  fi

  info "Starting bot in foreground (Ctrl+C to stop)"
  "$BIN_PATH"
}

start_background() {
  banner
  ensure_dirs

  if [[ ! -x "$BIN_PATH" ]]; then
    warn "Binary not found. Downloading latest release first."
    build_binary
  fi

  if [[ -f "$PID_FILE" ]]; then
    local pid
    pid="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      warn "Already running in background (pid $pid)"
      pause
      return
    fi
  fi

  nohup "$BIN_PATH" >>"$LOG_FILE" 2>&1 &
  echo $! >"$PID_FILE"
  sleep 1

  local new_pid
  new_pid="$(cat "$PID_FILE")"
  if kill -0 "$new_pid" >/dev/null 2>&1; then
    success "Background process started (pid $new_pid)"
    info "Log file: $LOG_FILE"
  else
    error "Failed to start background process"
  fi

  pause
}

stop_background() {
  banner
  if [[ ! -f "$PID_FILE" ]]; then
    warn "No PID file found."
    pause
    return
  fi

  local pid
  pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  if [[ -z "$pid" ]]; then
    warn "PID file is empty."
    rm -f "$PID_FILE"
    pause
    return
  fi

  if kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" || true
    sleep 1
    if kill -0 "$pid" >/dev/null 2>&1; then
      warn "Process still alive, sending SIGKILL"
      kill -9 "$pid" || true
    fi
    success "Stopped process $pid"
  else
    warn "Process $pid is not running"
  fi

  rm -f "$PID_FILE"
  pause
}

tail_background_logs() {
  banner
  if [[ ! -f "$LOG_FILE" ]]; then
    warn "No log file found at $LOG_FILE"
    pause
    return
  fi
  info "Tailing log: $LOG_FILE"
  tail -n 100 -f "$LOG_FILE"
}

install_systemd_service() {
  banner
  require_cmd systemctl || { pause; return; }

  if [[ ! -x "$BIN_PATH" ]]; then
    warn "Binary not found. Downloading latest release first."
    build_binary
  fi

  ensure_env_file

  local service_path="/etc/systemd/system/$SERVICE_NAME"
  local run_user
  run_user="$(id -un)"

  local tmp_file
  tmp_file="$(mktemp)"

  cat >"$tmp_file" <<SERVICE
[Unit]
Description=Hosibot Go Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$run_user
WorkingDirectory=$SCRIPT_DIR
EnvironmentFile=$ENV_FILE
ExecStart=$BIN_PATH
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SERVICE

  run_root cp "$tmp_file" "$service_path"
  rm -f "$tmp_file"

  run_root systemctl daemon-reload
  run_root systemctl enable "$SERVICE_NAME"

  success "Installed systemd service: $SERVICE_NAME"
  info "Service file: $service_path"
  pause
}

remove_systemd_service() {
  banner
  require_cmd systemctl || { pause; return; }

  local service_path="/etc/systemd/system/$SERVICE_NAME"
  run_root systemctl disable --now "$SERVICE_NAME" || true
  run_root rm -f "$service_path"
  run_root systemctl daemon-reload
  success "Removed systemd service: $SERVICE_NAME"
  pause
}

service_action() {
  local action="$1"
  require_cmd systemctl || return 1
  run_root systemctl "$action" "$SERVICE_NAME"
}

service_status() {
  banner
  require_cmd systemctl || { pause; return; }
  run_root systemctl status "$SERVICE_NAME" --no-pager || true
  pause
}

service_logs() {
  banner
  require_cmd journalctl || { pause; return; }
  run_root journalctl -u "$SERVICE_NAME" -n 100 -f
}

telegram_api_post() {
  local method="$1"
  shift
  load_env
  if [[ -z "${BOT_TOKEN:-}" ]]; then
    error "BOT_TOKEN is empty in .env"
    return 1
  fi

  local url="https://api.telegram.org/bot${BOT_TOKEN}/${method}"
  curl -sS --max-time 25 -X POST "$url" "$@"
}

print_json() {
  if command -v jq >/dev/null 2>&1; then
    jq .
  else
    cat
  fi
}

telegram_get_me() {
  banner
  telegram_api_post "getMe" | print_json
  pause
}

telegram_get_webhook() {
  banner
  telegram_api_post "getWebhookInfo" | print_json
  pause
}

telegram_set_webhook() {
  banner
  load_env
  local url="${BOT_WEBHOOK_URL:-}"

  if [[ -z "$url" ]]; then
    local suggested
    suggested="$(build_default_webhook "$(normalize_domain "${BOT_DOMAIN:-}")" "${BOT_TOKEN:-}")"
    url="$suggested"
  fi

  read -r -p "Webhook URL [$url]: " input
  url="${input:-$url}"

  if [[ -z "$url" ]]; then
    error "Webhook URL cannot be empty"
    pause
    return
  fi

  telegram_api_post "setWebhook" --data-urlencode "url=$url" | print_json

  if confirm "Save this URL to BOT_WEBHOOK_URL in .env?" "Y"; then
    set_env_value "BOT_WEBHOOK_URL" "$url"
    success "Updated BOT_WEBHOOK_URL in .env"
  fi

  pause
}

telegram_delete_webhook() {
  banner
  telegram_api_post "deleteWebhook" | print_json
  pause
}

health_check() {
  banner
  load_env

  local port="${APP_PORT:-8080}"
  local health_url="http://127.0.0.1:${port}/health"

  info "Environment summary"
  printf "APP_PORT=%s\n" "${APP_PORT:-}"
  printf "DB_HOST=%s DB_PORT=%s DB_NAME=%s\n" "${DB_HOST:-}" "${DB_PORT:-}" "${DB_NAME:-}"
  printf "REDIS_ADDR=%s REDIS_DB=%s\n" "${REDIS_ADDR:-}" "${REDIS_DB:-}"
  printf "BOT_DOMAIN=%s\n" "${BOT_DOMAIN:-}"
  printf "BOT_WEBHOOK_URL=%s\n" "${BOT_WEBHOOK_URL:-}"
  printf "BOT_TOKEN=%s\n" "$(mask_value "${BOT_TOKEN:-}")"
  line

  print_runtime_status
  print_service_status
  line

  if command -v curl >/dev/null 2>&1; then
    info "Health endpoint: $health_url"
    if curl -sS --max-time 5 "$health_url" >/tmp/hosibot_health.txt 2>/dev/null; then
      success "Health check reachable"
      cat /tmp/hosibot_health.txt
    else
      warn "Health endpoint not reachable"
    fi
  else
    warn "curl not found; skipped health request"
  fi

  if command -v mysql >/dev/null 2>&1 && [[ -n "${DB_HOST:-}" && -n "${DB_USER:-}" && -n "${DB_NAME:-}" ]]; then
    info "Testing MySQL connection"
    if MYSQL_PWD="${DB_PASS:-}" mysql -h "${DB_HOST}" -P "${DB_PORT:-3306}" -u "${DB_USER}" -D "${DB_NAME}" -e "SELECT 1;" >/dev/null 2>&1; then
      success "MySQL connection OK"
    else
      warn "MySQL connection failed"
    fi
  else
    warn "mysql client not installed or DB config incomplete"
  fi

  if command -v redis-cli >/dev/null 2>&1; then
    local redis_host redis_port redis_ping
    redis_host="${REDIS_ADDR%%:*}"
    redis_port="${REDIS_ADDR##*:}"
    if [[ -z "$redis_host" || "$redis_host" == "$redis_port" ]]; then
      redis_host="127.0.0.1"
      redis_port="6379"
    fi

    info "Testing Redis connection"
    if [[ -n "${REDIS_PASS:-}" ]]; then
      redis_ping="$(redis-cli -h "$redis_host" -p "$redis_port" -a "${REDIS_PASS}" -n "${REDIS_DB:-0}" ping 2>/dev/null || true)"
    else
      redis_ping="$(redis-cli -h "$redis_host" -p "$redis_port" -n "${REDIS_DB:-0}" ping 2>/dev/null || true)"
    fi

    if [[ "$redis_ping" == "PONG" ]]; then
      success "Redis connection OK"
    else
      warn "Redis connection failed"
    fi
  else
    warn "redis-cli not installed; skipped Redis check"
  fi

  rm -f /tmp/hosibot_health.txt
  pause
}

backup_env() {
  banner
  ensure_env_file
  local stamp
  stamp="$(date +%Y%m%d_%H%M%S)"
  local backup="$SCRIPT_DIR/.env.backup.$stamp"
  cp "$ENV_FILE" "$backup"
  success "Backup created: $backup"
  pause
}

quick_deploy() {
  banner
  info "Quick deploy sequence"

  if confirm "Install dependencies first?" "Y"; then
    install_dependencies
  fi

  configure_env_wizard

  if confirm "Bootstrap database and grants with current .env values?" "Y"; then
    bootstrap_database
  fi

  build_binary

  if confirm "Install/update systemd service?" "Y"; then
    install_systemd_service
  fi

  if confirm "Start systemd service now?" "Y"; then
    service_action start || true
  fi

  if confirm "Set Telegram webhook now?" "Y"; then
    telegram_set_webhook
  fi

  health_check
}

background_menu() {
  while true; do
    banner
    printf "%b\n" "${C_BOLD}Background Process Menu${C_RESET}"
    line
    print_runtime_status
    line
    cat <<MENU
1) Start background process
2) Stop background process
3) Tail background logs
4) Back
MENU

    read -r -p "Select: " ch
    case "$ch" in
      1) start_background ;;
      2) stop_background ;;
      3) tail_background_logs ;;
      4) return ;;
      *) warn "Invalid choice"; pause ;;
    esac
  done
}

service_menu() {
  while true; do
    banner
    printf "%b\n" "${C_BOLD}Systemd Service Menu (${SERVICE_NAME})${C_RESET}"
    line
    print_service_status
    line
    cat <<MENU
1) Install/Update service
2) Start service
3) Stop service
4) Restart service
5) Service status
6) Service logs (journalctl)
7) Remove service
8) Back
MENU

    read -r -p "Select: " ch
    case "$ch" in
      1) install_systemd_service ;;
      2) service_action start; pause ;;
      3) service_action stop; pause ;;
      4) service_action restart; pause ;;
      5) service_status ;;
      6) service_logs ;;
      7) remove_systemd_service ;;
      8) return ;;
      *) warn "Invalid choice"; pause ;;
    esac
  done
}

telegram_menu() {
  while true; do
    banner
    printf "%b\n" "${C_BOLD}Telegram Menu${C_RESET}"
    line
    cat <<MENU
1) getMe
2) getWebhookInfo
3) setWebhook
4) deleteWebhook
5) Back
MENU

    read -r -p "Select: " ch
    case "$ch" in
      1) telegram_get_me ;;
      2) telegram_get_webhook ;;
      3) telegram_set_webhook ;;
      4) telegram_delete_webhook ;;
      5) return ;;
      *) warn "Invalid choice"; pause ;;
    esac
  done
}

main_menu() {
  ensure_dirs
  while true; do
    banner
    print_runtime_status
    print_service_status
    line
    cat <<MENU
1) Quick deploy (recommended)
2) Configure .env wizard
3) Install dependencies
4) Download latest release binary
5) Run in foreground
6) Background process manager
7) Systemd service manager
8) Telegram webhook manager
9) Diagnostics / health check
10) Bootstrap database (DB_NAME/DB_USER/DB_PASS)
11) Backup .env
12) Exit
MENU

    read -r -p "Select: " choice
    case "$choice" in
      1) quick_deploy ;;
      2) configure_env_wizard ;;
      3) install_dependencies ;;
      4) build_binary ;;
      5) run_foreground ;;
      6) background_menu ;;
      7) service_menu ;;
      8) telegram_menu ;;
      9) health_check ;;
      10) bootstrap_database ;;
      11) backup_env ;;
      12) exit 0 ;;
      *) warn "Invalid choice"; pause ;;
    esac
  done
}

usage() {
  cat <<USAGE
Usage: $(basename "$0") [option]

Options:
  --menu           Open interactive menu (default)
  --quick-deploy   Run quick deploy flow
  --build          Download latest release binary
  --bootstrap-db   Create DB/user/grants from .env and verify connection
  --health         Run diagnostics
  --set-webhook    Configure Telegram webhook
  --help           Show this help
USAGE
}

case "${1:---menu}" in
  --menu) main_menu ;;
  --quick-deploy) quick_deploy ;;
  --build) build_binary ;;
  --bootstrap-db) bootstrap_database ;;
  --health) health_check ;;
  --set-webhook) telegram_set_webhook ;;
  --help|-h) usage ;;
  *) error "Unknown option: $1"; usage; exit 1 ;;
esac
