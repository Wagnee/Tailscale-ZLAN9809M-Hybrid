#!/bin/sh

# Instalador para o firmware ZLAN9809M/OpenWrt 21.02 customizado.
# Nao usa opkg: os feeds deste equipamento sao inconsistentes e o ABI do kernel
# nao corresponde ao repositorio oficial do OpenWrt.

set -u

BASE_URL="${ZLAN_RELEASE_URL:-https://raw.githubusercontent.com/Wagnee/Tailscale-ZLAN9809M-Hybrid/main/release}"
ARCHIVE_URL="$BASE_URL/zlan-hybrid-persistent.tar.gz"
HASH_URL="$ARCHIVE_URL.sha256"
WORK="/tmp/zlan-hybrid-install.$$"
ARCHIVE="$WORK/persistent.tar.gz"
HASH_FILE="$WORK/persistent.tar.gz.sha256"
PAYLOAD="$WORK/payload"
UPDATE=0
NEW_TAILSCALE_CONFIG=0

[ "${1:-}" = "--update" ] && UPDATE=1

cleanup() {
    rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

fail() {
    echo "ERRO: $*" >&2
    exit 1
}

get_release_value() {
    key="$1"
    sed -n "s/^${key}=['\"]\([^'\"]*\)['\"].*/\1/p" /etc/openwrt_release 2>/dev/null | sed -n '1p'
}

check_hardware() {
    release="$(get_release_value DISTRIB_RELEASE)"
    target="$(get_release_value DISTRIB_TARGET)"
    cpuinfo="$(cat /proc/cpuinfo 2>/dev/null)"
    machine="$(cat /proc/device-tree/model 2>/dev/null)"

    [ "$(id -u 2>/dev/null)" = "0" ] || fail "execute o instalador como root"
    [ "$release" = "21.02.0" ] || fail "OpenWrt $release nao suportado (esperado 21.02.0)"
    [ "$target" = "ramips/mt76x8" ] || fail "target $target nao suportado (esperado ramips/mt76x8)"
    echo "$cpuinfo $machine" | grep -qi -e 'MT7628' -e 'MIPS 24KEc' || fail "CPU MT7628AN/MIPS 24KEc nao detectada"
    [ -c /dev/net/tun ] || fail "/dev/net/tun nao existe; este firmware precisa trazer TUN no kernel"

    command -v wget >/dev/null 2>&1 || fail "wget nao encontrado"
    command -v sha256sum >/dev/null 2>&1 || fail "sha256sum nao encontrado"
    command -v tar >/dev/null 2>&1 || fail "tar nao encontrado"
    command -v uci >/dev/null 2>&1 || fail "uci nao encontrado"

    mem_total="$(sed -n 's/^MemTotal:[[:space:]]*\([0-9][0-9]*\).*/\1/p' /proc/meminfo)"
    if [ -z "$mem_total" ] || [ "$mem_total" -lt 110000 ]; then
        fail "RAM insuficiente (${mem_total:-0} KB; minimo 110000 KB)"
    fi

    overlay_free="$(df -k /overlay 2>/dev/null | awk 'NR > 1 { value=$4 } END { print value }')"
    if [ -z "$overlay_free" ] || [ "$overlay_free" -lt 2500 ]; then
        fail "espaco livre insuficiente no overlay (${overlay_free:-0} KB; minimo 2500 KB)"
    fi
}

verify_archive() {
    expected="$(sed -n 's/^\([0-9a-fA-F][0-9a-fA-F]*\).*/\1/p' "$HASH_FILE" | sed -n '1p' | tr 'A-F' 'a-f')"
    actual="$(sha256sum "$ARCHIVE" | sed 's/[[:space:]].*//' | tr 'A-F' 'a-f')"
    [ "${#expected}" = "64" ] || fail "arquivo SHA-256 invalido"
    [ "$actual" = "$expected" ] || fail "SHA-256 do pacote nao confere"
}

stop_legacy() {
    for init in zlan-tailscale zlan-telemetry tailscale-loader tailscale mqtt-daemon modbus-daemon cpufreq-manager auto-update; do
        if [ -x "/etc/init.d/$init" ]; then
            "/etc/init.d/$init" stop >/dev/null 2>&1 || true
            "/etc/init.d/$init" disable >/dev/null 2>&1 || true
        fi
    done
    killall tailscaled 2>/dev/null || true
    killall tailscale.combined 2>/dev/null || true
}

remove_legacy() {
    # Somente artefatos conhecidos dos dois projetos anteriores. Estado e configs sao preservados.
    rm -f /etc/init.d/tailscale-loader /etc/init.d/tailscale
    rm -f /etc/init.d/mqtt-daemon /etc/init.d/modbus-daemon
    rm -f /etc/init.d/cpufreq-manager /etc/init.d/auto-update
    rm -f /usr/bin/tailscale-loader.sh /usr/bin/tailscale-loader
    rm -f /usr/sbin/tailscale /usr/sbin/tailscaled /usr/sbin/tailscaled.xz
    rm -f /usr/bin/mqtt-daemon /usr/bin/modbus-daemon
    rm -f /usr/bin/cpu-governor-manager.sh
    rm -rf /usr/lib/auto-update
    rm -f /etc/auto-update-whitelist.conf
    rm -f /etc/hotplug.d/iface/99-tailscale /etc/uci-defaults/99-tailscale
    rm -f /usr/lib/lua/luci/controller/admin/tailscale.lua
    rm -f /usr/lib/lua/luci/controller/admin/tailscale_status.lua
    rm -f /usr/lib/lua/luci/controller/admin/modbus.lua
    rm -f /usr/lib/lua/luci/controller/admin/mqtt.lua
    rm -f /usr/lib/lua/luci/controller/cpufreq.lua
    rm -f /usr/lib/lua/luci/model/cbi/tailscale.lua
    rm -f /usr/lib/lua/luci/model/cbi/modbus.lua
    rm -f /usr/lib/lua/luci/model/cbi/mqtt.lua
    rm -f /usr/lib/lua/luci/model/cbi/cpufreq.lua
    rm -rf /usr/lib/lua/luci/view/tailscale /usr/lib/lua/luci/view/modbus
    rm -rf /usr/lib/lua/luci/view/mqtt /usr/lib/lua/luci/view/cpufreq
    rm -f /tmp/tailscale /tmp/tailscaled /tmp/tailscale.combined.part
}

install_defaults() {
    mkdir -p /etc/config /etc/tailscale
    chmod 0700 /etc/tailscale
    for config in zlan_tailscale zlan_modbus zlan_mqtt; do
        if [ ! -f "/etc/config/$config" ]; then
            cp "/usr/share/zlan-hybrid/defaults/$config" "/etc/config/$config"
            [ "$config" != "zlan_tailscale" ] || NEW_TAILSCALE_CONFIG=1
        fi
        chmod 0600 "/etc/config/$config"
    done
}

configure_detected_lan_route() {
    [ "$NEW_TAILSCALE_CONFIG" = "1" ] || return 0
    lan_ip="$(uci -q get network.lan.ipaddr 2>/dev/null)"
    lan_mask="$(uci -q get network.lan.netmask 2>/dev/null)"
    if [ "$lan_mask" = "255.255.255.0" ]; then
        case "$lan_ip" in
            *.*.*.*)
                detected_route="${lan_ip%.*}.0/24"
                uci set "zlan_tailscale.main.routes=$detected_route"
                uci commit zlan_tailscale
                echo "Rota LAN detectada: $detected_route"
                ;;
        esac
    fi
}

configure_first_install() {
    [ "$UPDATE" = "0" ] || return 0
    [ -t 0 ] || return 0

    current_hostname="$(uci -q get zlan_tailscale.main.hostname 2>/dev/null)"
    current_routes="$(uci -q get zlan_tailscale.main.routes 2>/dev/null)"
    printf 'Hostname Tailscale [%s]: ' "$current_hostname"
    read -r value
    [ -z "$value" ] || uci set "zlan_tailscale.main.hostname=$value"
    printf 'Rota LAN anunciada [%s]: ' "$current_routes"
    read -r value
    [ -z "$value" ] || uci set "zlan_tailscale.main.routes=$value"
    printf 'Auth key Tailscale (opcional, entrada oculta): '
    stty -echo 2>/dev/null || true
    read -r value
    stty echo 2>/dev/null || true
    echo
    [ -z "$value" ] || uci set "zlan_tailscale.main.auth_key=$value"
    uci commit zlan_tailscale
}

echo "=========================================="
echo "ZLAN9809M Hybrid - instalacao persistente"
echo "=========================================="
echo "Validando hardware e firmware..."
check_hardware

mkdir -p "$WORK" "$PAYLOAD" || fail "nao foi possivel criar $WORK"
echo "Baixando pacote persistente..."
wget -O "$ARCHIVE" "$ARCHIVE_URL" || fail "falha no download do pacote"
wget -O "$HASH_FILE" "$HASH_URL" || fail "falha no download do SHA-256"
verify_archive
tar -xzf "$ARCHIVE" -C "$PAYLOAD" || fail "pacote corrompido"
[ -f "$PAYLOAD/usr/bin/zlan-tailscale-loader" ] || fail "payload incompleto"

echo "Instalando componentes na flash..."
stop_legacy
remove_legacy
cp -R "$PAYLOAD"/. / || fail "falha copiando arquivos para a flash"
chmod 0755 /etc/init.d/zlan-tailscale /etc/init.d/zlan-telemetry
chmod 0755 /usr/bin/zlan-tailscale-loader /usr/bin/zlan-telemetryd
chmod 0755 /usr/bin/zlan-hybrid-status /usr/bin/zlan-hybrid-cleanup
chmod 0755 /usr/bin/zlan-hybrid-update /usr/bin/zlan-hybrid-uninstall /usr/bin/zlan-system
chmod 0644 /usr/lib/lua/luci/controller/zlan_hybrid.lua
chmod 0644 /usr/lib/lua/luci/model/cbi/zlan_tailscale.lua
chmod 0644 /usr/lib/lua/luci/model/cbi/zlan_modbus.lua
chmod 0644 /usr/lib/lua/luci/model/cbi/zlan_mqtt.lua
chmod 0755 /usr/lib/lua/luci/view/zlan_hybrid
find /usr/lib/lua/luci/view/zlan_hybrid -type f -exec chmod 0644 {} \;
find /usr/share/zlan-hybrid -type d -exec chmod 0755 {} \;
find /usr/share/zlan-hybrid -type f -exec chmod 0644 {} \;

install_defaults
configure_detected_lan_route
configure_first_install
mkdir -p /etc/sysctl.d
printf '%s\n' 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-zlan-hybrid.conf
echo 1 > /proc/sys/net/ipv4/ip_forward 2>/dev/null || true

/etc/init.d/zlan-tailscale enable
/etc/init.d/zlan-telemetry enable
rm -f /tmp/luci-indexcache

/etc/init.d/zlan-tailscale start
/etc/init.d/zlan-telemetry start

echo ""
echo "Instalacao concluida. O tailscale.combined sera baixado para /tmp quando houver internet."
echo "Estado: zlan-hybrid-status"
echo "LuCI: Servicos > ZLAN Hybrid"
