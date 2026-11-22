#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY=alexeeeeeeey/VPNPanel
INSTALL_DIR=/opt/vpnpanel
CONFIG_DIR=/etc/vpnpanel
DATA_DIR=/var/lib/vpnpanel

fail() { printf 'Ошибка: %s\n' "$*" >&2; exit 1; }

architecture() {
    case "$1" in
        x86_64 | amd64) printf 'amd64\n' ;;
        aarch64 | arm64) printf 'arm64\n' ;;
        *) printf 'Неподдерживаемая архитектура: %s\n' "$1" >&2; return 1 ;;
    esac
}

install_dependencies() {
    if command -v curl >/dev/null && command -v openssl >/dev/null &&
        command -v sha256sum >/dev/null && command -v useradd >/dev/null; then
        return
    fi
    if command -v apt-get >/dev/null; then
        apt-get update
        DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl openssl coreutils passwd
    elif command -v dnf >/dev/null; then
        dnf install -y ca-certificates curl openssl coreutils shadow-utils
    elif command -v yum >/dev/null; then
        yum install -y ca-certificates curl openssl coreutils shadow-utils
    else
        fail 'Установите ca-certificates, curl, openssl, sha256sum и useradd, затем повторите запуск.'
    fi
}

download() {
    curl --fail --silent --show-error --location --retry 3 \
        --connect-timeout 15 --max-time 180 --proto '=https' --proto-redir '=https' \
        "$1" -o "$2"
}

verify_checksum() {
    local directory=$1 name=$2 entry
    entry=$(awk -v name="$name" '$2 == name { print }' "$directory/SHA256SUMS")
    [[ $(printf '%s\n' "$entry" | wc -l) -eq 1 && -n "$entry" ]] || {
        printf 'В SHA256SUMS нет единственной записи для %s\n' "$name" >&2
        return 1
    }
    (cd "$directory" && printf '%s\n' "$entry" | sha256sum --check --status)
}

# Existing secrets are retained on every subsequent installation.
prepare_access() {
    local directory=$1
    install -d -m 700 "$directory/access"
    if [[ ! -e "$directory/access/proxy.key" ]]; then
        openssl rand -hex 32 > "$directory/access/proxy.key"
    fi
    if [[ ! -e "$directory/panel.env" ]]; then
        printf 'VPNPANEL_MTLS_PROXY_KEY_FILE=%s/access/proxy.key\nVPNPANEL_NODE_KEY=%s\n' \
            "$directory" "$(openssl rand -hex 32)" > "$directory/panel.env"
    fi
    if [[ ! -e "$directory/access/cli.headers" ]]; then
        printf 'X-Panel-Proxy-Key: %s\nX-Panel-CLI: 1\n' \
            "$(tr -d '\r\n' < "$directory/access/proxy.key")" > "$directory/access/cli.headers"
    fi
    chmod 600 "$directory/access/proxy.key" "$directory/access/cli.headers" "$directory/panel.env"
}

render_unit() {
    cat <<'UNIT'
[Unit]
Description=VPNPanel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vpnpanel
Group=vpnpanel
WorkingDirectory=/var/lib/vpnpanel
EnvironmentFile=/etc/vpnpanel/panel.env
ExecStart=/opt/vpnpanel/vpnpanel -listen 127.0.0.1:9080 -db /var/lib/vpnpanel/panel.db -base-path /panel
Restart=on-failure
RestartSec=3
TimeoutStopSec=30
UMask=0077

[Install]
WantedBy=multi-user.target
UNIT
}

main() {
    [[ $(uname -s) == Linux ]] || fail 'Установщик поддерживает только Linux.'
    [[ $EUID -eq 0 ]] || fail 'Запустите установщик от root или через sudo.'
    command -v systemctl >/dev/null && [[ -d /run/systemd/system ]] || fail 'Нужен сервер с работающим systemd.'
    local arch version release_url base temporary panel cli
    arch=$(architecture "$(uname -m)") || exit 1
    install_dependencies
    umask 077
    temporary=$(mktemp -d)
    trap "$(printf 'rm -rf -- %q' "$temporary")" EXIT

    printf 'Проверка последнего релиза %s…\n' "$REPOSITORY"
    release_url=$(curl --fail --silent --show-error --head --location \
        --connect-timeout 15 --max-time 60 --proto '=https' --proto-redir '=https' \
        --output /dev/null --write-out '%{url_effective}' "https://github.com/$REPOSITORY/releases/latest") || \
        fail 'Не удалось найти релиз. Репозиторий должен быть публичным и содержать опубликованный релиз.'
    [[ $release_url == "https://github.com/$REPOSITORY/releases/tag/"* ]] || fail 'Некорректный адрес релиза.'
    version=${release_url##*/}
    [[ $version =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail 'Некорректный тег стабильного релиза.'
    base="https://github.com/$REPOSITORY/releases/download/$version"
    panel="vpnpanel-linux-$arch"
    cli="vpnpanelctl-linux-$arch"
    download "$base/SHA256SUMS" "$temporary/SHA256SUMS"
    download "$base/$panel" "$temporary/$panel"
    download "$base/$cli" "$temporary/$cli"
    verify_checksum "$temporary" "$panel" || fail 'Не совпадает SHA-256 панели.'
    verify_checksum "$temporary" "$cli" || fail 'Не совпадает SHA-256 CLI.'
    chmod 755 "$temporary/$panel" "$temporary/$cli"
    [[ $("$temporary/$panel" -version) == "$version" ]] || fail 'Бинарник панели не подтвердил версию.'
    [[ $("$temporary/$cli" -version) == "$version" ]] || fail 'Бинарник CLI не подтвердил версию.'

    local unit=/etc/systemd/system/vpnpanel.service
    if [[ -e "$unit" ]] && ! grep -Eq '^ExecStart=/opt/vpnpanel/vpnpanel([[:space:]]|$)' "$unit"; then
        fail 'В существующей службе другой путь к бинарнику. Измените ExecStart на /opt/vpnpanel/vpnpanel, сохранив параметры, и повторите установку.'
    fi

    if ! id vpnpanel >/dev/null 2>&1; then
        useradd --system --user-group --home-dir "$DATA_DIR" --shell /usr/sbin/nologin vpnpanel
    fi
    install -d -o vpnpanel -g vpnpanel -m 755 "$INSTALL_DIR"
    install -d -o vpnpanel -g vpnpanel -m 700 "$DATA_DIR"
    install -d -m 755 "$CONFIG_DIR"
    prepare_access "$CONFIG_DIR"
    chown vpnpanel:vpnpanel "$CONFIG_DIR/access" "$CONFIG_DIR/access/proxy.key"

    if [[ ! -e "$unit" ]]; then
        render_unit > "$unit"
        chmod 644 "$unit"
    else
        printf 'Существующая служба %s сохранена.\n' "$unit"
    fi
    install -o vpnpanel -g vpnpanel -m 755 "$temporary/$panel" "$INSTALL_DIR/.vpnpanel-new"
    install -m 755 "$temporary/$cli" "$INSTALL_DIR/.vpnpanelctl-new"
    systemctl daemon-reload
    systemctl stop vpnpanel.service
    mv -f "$INSTALL_DIR/.vpnpanel-new" "$INSTALL_DIR/vpnpanel"
    mv -f "$INSTALL_DIR/.vpnpanelctl-new" "$INSTALL_DIR/vpnpanelctl"
    ln -sfn "$INSTALL_DIR/vpnpanelctl" /usr/local/bin/vpnpanelctl
    systemctl enable --now vpnpanel.service
    systemctl is-active --quiet vpnpanel.service || fail 'Служба не запустилась. Проверьте journalctl -u vpnpanel.'

    printf '\nVPNPanel %s установлен.\n' "$version"
    printf 'Бинарник: %s/vpnpanel\nБаза: %s/panel.db\n' "$INSTALL_DIR" "$DATA_DIR"
    printf 'Ключи: %s/panel.env и %s/access/proxy.key\n' "$CONFIG_DIR" "$CONFIG_DIR"
    printf 'CLI-заголовки: %s/access/cli.headers\n' "$CONFIG_DIR"
    printf 'Состояние: systemctl status vpnpanel\nЛоги: journalctl -u vpnpanel -f\n'
    printf '\nBackend слушает 127.0.0.1:9080 с путём /panel.\n'
    printf 'Для браузера настройте nginx и клиентский сертификат по разделу «Доступ к панели» в README.\n'
    printf 'Xray устанавливается отдельно; параметры -xray и -xray-api задаются в службе.\n'
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
    main "$@"
fi
