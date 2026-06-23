module("luci.controller.zlan_hybrid", package.seeall)

function index()
    entry({"admin", "services", "zlan_hybrid"}, template("zlan_hybrid/overview"), _("ZLAN Hybrid"), 40).dependent = false
    entry({"admin", "services", "zlan_hybrid", "tailscale"}, cbi("zlan_tailscale"), _("Tailscale"), 10)
    entry({"admin", "services", "zlan_hybrid", "modbus"}, cbi("zlan_modbus"), _("Modbus"), 20)
    entry({"admin", "services", "zlan_hybrid", "mqtt"}, cbi("zlan_mqtt"), _("MQTT"), 30)
    entry({"admin", "services", "zlan_hybrid", "action"}, post("action_service")).leaf = true
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
