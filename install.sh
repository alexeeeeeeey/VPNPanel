#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY=alexeeeeeeey/VPNPanel
INSTALL_DIR=/opt/vpnpanel
CONFIG_DIR=/etc/vpnpanel
DATA_DIR=/var/lib/vpnpanel

fail() { printf 'Ошибка: %s\n' "$*" >&2; exit 1; }

ask() {
    local label=$1 fallback=$2 answer
    printf '%s [%s]: ' "$label" "$fallback" >&2
    IFS= read -r answer < /dev/tty || fail 'Не удалось прочитать ответ из терминала.'
    printf '%s' "${answer:-$fallback}"
}

valid_port() {
    [[ $1 =~ ^[0-9]{1,5}$ ]] && ((10#$1 >= 1 && 10#$1 <= 65535))
}

valid_path() {
    [[ -z $1 || $1 =~ ^(/[a-zA-Z0-9_-]+)+$ ]]
}

valid_host() {
    [[ $1 =~ ^([a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?|\[[a-fA-F0-9:]+\])$ ]]
}

valid_listen() {
    local address=$1 octet
    if [[ $address =~ ^127\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3}):([0-9]{1,5})$ ]]; then
        for octet in "${BASH_REMATCH[@]:1:3}"; do
            [[ $octet == 0 || $octet != 0* ]] || return 1
        done
        ((10#${BASH_REMATCH[1]} <= 255 && 10#${BASH_REMATCH[2]} <= 255 && 10#${BASH_REMATCH[3]} <= 255)) || return 1
        valid_port "${BASH_REMATCH[4]}"
    elif [[ $address =~ ^\[::1\]:([0-9]{1,5})$ ]]; then
        valid_port "${BASH_REMATCH[1]}"
    else
        return 1
    fi
}

# Join an ExecStart split over several lines without evaluating shell content.
service_command() {
    awk '
        /^ExecStart=/ {
            sub(/^ExecStart=/, "")
            command = $0
            while (command ~ /\\$/ && getline > 0) {
                sub(/\\$/, "", command)
                command = command " " $0
            }
            print command
            exit
        }
    ' "$1"
}

configure_service() {
    local unit=$1 listen=$2 prefix=$3 xray=$4 xray_api=$5 command temporary
    command=$(service_command "$unit")
    command=$(printf '%s\n' "$command" | sed -E \
        -e 's/(^|[[:space:]])-listen(=|[[:space:]]+)("[^"]*"|[^[:space:]]+)//g' \
        -e 's/(^|[[:space:]])-base-path(=|[[:space:]]+)("[^"]*"|[^[:space:]]+)//g' \
        -e 's/(^|[[:space:]])-xray-api(=|[[:space:]]+)("[^"]*"|[^[:space:]]+)//g' \
        -e 's/(^|[[:space:]])-xray(=|[[:space:]]+)("[^"]*"|[^[:space:]]+)//g')
    command="$command -listen $listen -base-path \"$prefix\" -xray \"$xray\" -xray-api $xray_api"
    temporary=$(mktemp "${unit}.XXXXXX")
    VPNPANEL_EXEC_START="$command" awk '
        /^ExecStart=/ {
            continuing = ($0 ~ /\\$/)
            print "ExecStart=" ENVIRON["VPNPANEL_EXEC_START"]
            while (continuing && getline > 0) continuing = ($0 ~ /\\$/)
            next
        }
        { print }
    ' "$unit" > "$temporary"
    chmod 644 "$temporary"
    mv -f "$temporary" "$unit"
}

architecture() {
    case "$1" in
        x86_64 | amd64) printf 'amd64\n' ;;
        aarch64 | arm64) printf 'arm64\n' ;;
        *) printf 'Неподдерживаемая архитектура: %s\n' "$1" >&2; return 1 ;;
    esac
}

install_dependencies() {
    if command -v curl >/dev/null && command -v openssl >/dev/null &&
        command -v sha256sum >/dev/null && command -v useradd >/dev/null && command -v unzip >/dev/null; then
        return
    fi
    if command -v apt-get >/dev/null; then
        apt-get update
        DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl openssl coreutils passwd unzip
    elif command -v dnf >/dev/null; then
        dnf install -y ca-certificates curl openssl coreutils shadow-utils unzip
    elif command -v yum >/dev/null; then
        yum install -y ca-certificates curl openssl coreutils shadow-utils unzip
    else
        fail 'Установите ca-certificates, curl, openssl, sha256sum, useradd и unzip, затем повторите запуск.'
    fi
}

download() {
    curl --fail --silent --show-error --location --retry 3 \
        --connect-timeout 15 --max-time 180 --proto '=https' --proto-redir '=https' \
        "$1" -o "$2"
}

prepare_xray() {
    local arch=$1 temporary=$2 destination=$3 release_url version archive digest directory file
    if [[ -x "$destination" ]]; then
        "$destination" version > /dev/null || fail "Не удалось запустить существующий Xray: $destination"
        printf 'Используется установленный Xray: %s\n' "$destination"
        return
    fi
    printf 'Установка Xray из XTLS/Xray-core…\n'
    release_url=$(curl --fail --silent --show-error --head --location \
        --connect-timeout 15 --max-time 60 --proto '=https' --proto-redir '=https' \
        --output /dev/null --write-out '%{url_effective}' https://github.com/XTLS/Xray-core/releases/latest) || \
        fail 'Не удалось найти релиз Xray.'
    [[ $release_url == https://github.com/XTLS/Xray-core/releases/tag/* ]] || fail 'Некорректный адрес релиза Xray.'
    version=${release_url##*/}
    [[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Некорректный тег релиза Xray.'
    case "$arch" in
        amd64) archive=Xray-linux-64.zip ;;
        arm64) archive=Xray-linux-arm64-v8a.zip ;;
        *) fail 'Неподдерживаемая архитектура Xray.' ;;
    esac
    download "https://github.com/XTLS/Xray-core/releases/download/$version/$archive" "$temporary/$archive"
    download "https://github.com/XTLS/Xray-core/releases/download/$version/$archive.dgst" "$temporary/$archive.dgst"
    digest=$(awk '$1 == "SHA2-256=" || $1 == "SHA256=" { print $2 }' "$temporary/$archive.dgst")
    [[ $digest =~ ^[a-fA-F0-9]{64}$ ]] || fail 'В релизе Xray нет корректной SHA-256.'
    (cd "$temporary" && printf '%s  %s\n' "$digest" "$archive" | sha256sum --check --status) || fail 'Не совпадает SHA-256 Xray.'
    mkdir "$temporary/xray"
    unzip -q -j "$temporary/$archive" xray -d "$temporary/xray"
    chmod 755 "$temporary/xray/xray"
    "$temporary/xray/xray" version > /dev/null || fail 'Скачанный Xray не запускается.'
    directory=$(dirname -- "$destination")
    install -d -m 755 "$directory"
    install -m 755 "$temporary/xray/xray" "$directory/.xray-new"
    mv -f "$directory/.xray-new" "$destination"
    for file in geoip.dat geosite.dat; do
        if unzip -Z1 "$temporary/$archive" | grep -Fx "$file" > /dev/null; then
            unzip -q -j "$temporary/$archive" "$file" -d "$temporary/xray"
            if [[ ! -e "$directory/$file" ]]; then
                install -m 644 "$temporary/xray/$file" "$directory/$file"
            fi
        fi
    done
    printf 'Xray %s установлен: %s\n' "$version" "$destination"
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
ExecStart=/opt/vpnpanel/vpnpanel -listen 127.0.0.1:8090 -db /var/lib/vpnpanel/panel.db -base-path /panel
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
    command -v systemctl >/dev/null && command -v systemd-run >/dev/null && [[ -d /run/systemd/system ]] || fail 'Нужен сервер с работающим systemd.'
    [[ -r /dev/tty && -w /dev/tty ]] || fail 'Для выбора порта, пути и адреса нужен терминал.'
    local unit=/etc/systemd/system/vpnpanel.service
    local listen=127.0.0.1:8090 prefix=/panel origin='' host='' external_port=8090 command='' connection_url connection_code
    local xray=/usr/local/bin/xray xray_api=127.0.0.1:10085
    if command -v xray > /dev/null; then
        xray=$(command -v xray)
    fi
    if [[ -e "$unit" ]]; then
        command=$(service_command "$unit")
        [[ $command == "$INSTALL_DIR/vpnpanel" || $command == "$INSTALL_DIR/vpnpanel "* ]] || \
            fail 'В существующей службе другой путь к бинарнику. Измените ExecStart на /opt/vpnpanel/vpnpanel, сохранив параметры, и повторите установку.'
        if [[ $command =~ -listen[=[:space:]]+\"([^\"]*)\" ]]; then
            listen=${BASH_REMATCH[1]}
        elif [[ $command =~ -listen[=[:space:]]+([^[:space:]]+) ]]; then
            listen=${BASH_REMATCH[1]}
        fi
        if [[ $command =~ -base-path[=[:space:]]+\"([^\"]*)\" ]]; then
            prefix=${BASH_REMATCH[1]}
        elif [[ $command =~ -base-path[=[:space:]]+([^[:space:]]+) ]]; then
            prefix=${BASH_REMATCH[1]}
        fi
        if [[ $command =~ (^|[[:space:]])-xray[=[:space:]]+\"([^\"]*)\" ]]; then
            xray=${BASH_REMATCH[2]}
        elif [[ $command =~ (^|[[:space:]])-xray[=[:space:]]+([^[:space:]]+) ]]; then
            xray=${BASH_REMATCH[2]}
        fi
        if [[ $command =~ -xray-api[=[:space:]]+\"([^\"]*)\" ]]; then
            xray_api=${BASH_REMATCH[1]}
        elif [[ $command =~ -xray-api[=[:space:]]+([^[:space:]]+) ]]; then
            xray_api=${BASH_REMATCH[1]}
        fi
    fi
    xray=${xray:-/usr/local/bin/xray}
    xray_api=${xray_api:-127.0.0.1:10085}
    [[ $xray =~ ^/[a-zA-Z0-9_./-]+$ && $xray != */ && $xray != */.. && $xray != */. && $xray != */../* && $xray != */./* ]] || \
        fail 'В службе указан неподдерживаемый путь Xray.'
    valid_listen "$xray_api" || fail 'API Xray должен использовать loopback-адрес с портом.'
    if [[ -f "$CONFIG_DIR/public-origin" ]]; then
        origin=$(cat "$CONFIG_DIR/public-origin")
        if [[ $origin =~ ^https://(\[[a-fA-F0-9:]+\]|[a-zA-Z0-9.-]+)(:([0-9]{1,5}))?$ ]]; then
            host=${BASH_REMATCH[1]}
            external_port=${BASH_REMATCH[3]:-443}
        fi
    fi
    while true; do
        listen=$(ask 'Адрес прослушивания backend' "$listen")
        if valid_listen "$listen" && [[ $listen != "$xray_api" ]]; then break; fi
        printf 'Нужен loopback-адрес с портом от 1 до 65535, например 127.0.0.1:8090 или [::1]:8090.\n' >&2
        printf 'Адрес не должен совпадать с API Xray (%s).\n' "$xray_api" >&2
        listen=127.0.0.1:8090
    done
    while true; do
        host=$(ask 'Внешний хост (домен или IP, без https://, порта и пути)' "$host")
        valid_host "$host" && break
        printf 'Укажите домен или IP. Для IPv6 используйте квадратные скобки, например [2001:db8::1].\n' >&2
        host=''
    done
    while true; do
        external_port=$(ask 'Внешний HTTPS-порт' "$external_port")
        if valid_port "$external_port"; then external_port=$((10#$external_port)); break; fi
        printf 'Укажите порт от 1 до 65535.\n' >&2
        external_port=8090
    done
    while true; do
        prefix=$(ask 'Базовый путь панели (/ для корня)' "${prefix:-/}")
        [[ $prefix != / ]] || prefix=''
        valid_path "$prefix" && break
        printf 'Допустим путь вида /panel или /my/panel: латинские буквы, цифры, дефис и подчёркивание.\n' >&2
        prefix=/panel
    done
    origin="https://$host:$external_port"
    connection_url="$origin$prefix"
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
    prepare_xray "$arch" "$temporary" "$xray"

    if ! id vpnpanel >/dev/null 2>&1; then
        useradd --system --user-group --home-dir "$DATA_DIR" --shell /usr/sbin/nologin vpnpanel
    fi
    install -d -o vpnpanel -g vpnpanel -m 755 "$INSTALL_DIR"
    install -d -o vpnpanel -g vpnpanel -m 700 "$DATA_DIR"
    install -d -m 755 "$CONFIG_DIR"
    prepare_access "$CONFIG_DIR"
    if ! grep -Eq '^VPNPANEL_NODE_KEY=.+$' "$CONFIG_DIR/panel.env"; then
        printf '\nVPNPANEL_NODE_KEY=%s\n' "$(openssl rand -hex 32)" >> "$CONFIG_DIR/panel.env"
    fi
    printf '%s\n' "$origin" > "$CONFIG_DIR/public-origin"
    chown vpnpanel:vpnpanel "$CONFIG_DIR/access" "$CONFIG_DIR/access/proxy.key"

    if [[ ! -e "$unit" ]]; then
        render_unit > "$unit"
        chmod 644 "$unit"
    else
        printf 'Обновление адреса, пути и параметров Xray в %s.\n' "$unit"
    fi
    configure_service "$unit" "$listen" "$prefix" "$xray" "$xray_api"
    install -o vpnpanel -g vpnpanel -m 755 "$temporary/$panel" "$INSTALL_DIR/.vpnpanel-new"
    install -m 755 "$temporary/$cli" "$INSTALL_DIR/.vpnpanelctl-new"
    systemctl daemon-reload
    systemctl stop vpnpanel.service
    mv -f "$INSTALL_DIR/.vpnpanel-new" "$INSTALL_DIR/vpnpanel"
    mv -f "$INSTALL_DIR/.vpnpanelctl-new" "$INSTALL_DIR/vpnpanelctl"
    ln -sfn "$INSTALL_DIR/vpnpanelctl" /usr/local/bin/vpnpanelctl
    connection_code=$(systemd-run --quiet --wait --pipe --collect --uid=vpnpanel \
        --property="EnvironmentFile=$CONFIG_DIR/panel.env" \
        --property="WorkingDirectory=$DATA_DIR" --property=UMask=0077 \
        "$INSTALL_DIR/vpnpanel" -db "$DATA_DIR/panel.db" \
        -base-path "$prefix" -connection-url "$connection_url") || \
        fail 'Не удалось создать строку подключения узла.'
    systemctl enable --now vpnpanel.service
    systemctl is-active --quiet vpnpanel.service || fail 'Служба не запустилась. Проверьте journalctl -u vpnpanel.'

    printf '\nVPNPanel %s установлен.\n' "$version"
    printf 'Бинарник: %s/vpnpanel\nБаза: %s/panel.db\n' "$INSTALL_DIR" "$DATA_DIR"
    printf 'Ключи: %s/panel.env и %s/access/proxy.key\n' "$CONFIG_DIR" "$CONFIG_DIR"
    printf 'CLI-заголовки: %s/access/cli.headers\n' "$CONFIG_DIR"
    printf 'Состояние: systemctl status vpnpanel\nЛоги: journalctl -u vpnpanel -f\n'
    printf '\nBackend: %s, путь: %s\n' "$listen" "${prefix:-/}"
    printf 'Внешний URL панели: %s/\n' "$connection_url"
    printf '\nСтрока для добавления этого узла в разделе «Серверы» другой панели:\n%s\n' "$connection_code"
    printf 'Строка содержит API-ключ; храните её как секрет.\n\n'
    printf 'Для браузера настройте nginx и клиентский сертификат по разделу «Доступ к панели» в README.\n'
    printf 'Xray: %s\nAPI статистики: %s\n' "$xray" "$xray_api"
    printf 'В серверном шаблоне включите StatsService и пользовательскую статистику; пример есть в README.\n'
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
    main "$@"
fi
