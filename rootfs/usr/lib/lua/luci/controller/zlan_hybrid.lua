module("luci.controller.zlan_hybrid", package.seeall)

function index()
    entry({"admin", "services", "zlan_hybrid"}, template("zlan_hybrid/overview"), _("ZLAN Hybrid"), 40).dependent = false
    entry({"admin", "services", "zlan_hybrid", "tailscale"}, cbi("zlan_tailscale"), _("Tailscale"), 10)
    entry({"admin", "services", "zlan_hybrid", "modbus"}, cbi("zlan_modbus"), _("Modbus"), 20)
    entry({"admin", "services", "zlan_hybrid", "mqtt"}, cbi("zlan_mqtt"), _("MQTT"), 30)
    entry({"admin", "services", "zlan_hybrid", "action"}, post("action_service")).leaf = true
    entry({"admin", "services", "zlan_hybrid", "test_modbus"}, post("action_test_modbus")).leaf = true
    entry({"admin", "services", "zlan_hybrid", "test_mqtt_publish"}, post("action_test_mqtt_publish")).leaf = true
    entry({"admin", "services", "zlan_hybrid", "test_mqtt_subscribe"}, post("action_test_mqtt_subscribe")).leaf = true
end

local function run_test(command, result_path)
    local sys = require "luci.sys"
    local fs = require "nixio.fs"
    sys.call("mkdir -p /tmp/zlan-telemetry")
    local output = sys.exec(command .. " 2>&1")
    if output == "" then
        output = "Comando concluido sem saida.\n"
    end
    fs.writefile(result_path, os.date("%Y-%m-%d %H:%M:%S\n") .. output)
end

local function redirect_to(page)
    local http = require "luci.http"
    http.redirect(luci.dispatcher.build_url("admin", "services", "zlan_hybrid", page))
end

function action_service()
    local http = require "luci.http"
    local sys = require "luci.sys"
    local service = http.formvalue("service") or ""
    local action = http.formvalue("action") or ""
    local allowed_services = { tailscale = "zlan-tailscale", telemetry = "zlan-telemetry" }
    local allowed_actions = { start = true, stop = true, restart = true }
    local init_name = allowed_services[service]

    if init_name and allowed_actions[action] then
        sys.call("/etc/init.d/" .. init_name .. " " .. action .. " >/dev/null 2>&1")
    end
    http.redirect(luci.dispatcher.build_url("admin", "services", "zlan_hybrid"))
end

function action_test_modbus()
    local http = require "luci.http"
    local util = require "luci.util"
    local device = http.formvalue("device") or ""
    local address = tonumber(http.formvalue("address") or "")
    local value = http.formvalue("value") or ""

    if not device:match("^[%w_%-]+$") or not address or address < 0 or address > 65535 or address ~= math.floor(address) or (value ~= "0" and value ~= "1") then
        run_test("printf %s " .. util.shellquote("Parametros Modbus invalidos.\n"), "/tmp/zlan-telemetry/modbus-test.txt")
    else
        local command = "/usr/bin/zlan-telemetryd modbus-write-coil " ..
            util.shellquote(device) .. " " .. util.shellquote(string.format("%d", address)) .. " " .. util.shellquote(value)
        run_test(command, "/tmp/zlan-telemetry/modbus-test.txt")
    end
    redirect_to("modbus")
end

function action_test_mqtt_publish()
    local http = require "luci.http"
    local util = require "luci.util"
    local topic = http.formvalue("publish_topic") or ""
    local message = http.formvalue("publish_message") or ""

    if #topic < 1 or #topic > 256 or #message > 2048 then
        run_test("printf %s " .. util.shellquote("Topico ou mensagem MQTT invalidos.\n"), "/tmp/zlan-telemetry/mqtt-test.txt")
    else
        local command = "/usr/bin/zlan-telemetryd mqtt-publish " .. util.shellquote(topic) .. " " .. util.shellquote(message)
        run_test(command, "/tmp/zlan-telemetry/mqtt-test.txt")
    end
    redirect_to("mqtt")
end

function action_test_mqtt_subscribe()
    local http = require "luci.http"
    local util = require "luci.util"
    local topic = http.formvalue("subscribe_topic") or ""
    local timeout = tonumber(http.formvalue("subscribe_timeout") or "10")

    if #topic < 1 or #topic > 256 or not timeout or timeout < 1 or timeout > 15 or timeout ~= math.floor(timeout) then
        run_test("printf %s " .. util.shellquote("Topico ou timeout MQTT invalidos.\n"), "/tmp/zlan-telemetry/mqtt-test.txt")
    else
        local command = "/usr/bin/zlan-telemetryd mqtt-subscribe " ..
            util.shellquote(topic) .. " " .. util.shellquote(string.format("%d", timeout))
        run_test(command, "/tmp/zlan-telemetry/mqtt-test.txt")
    end
    redirect_to("mqtt")
end
