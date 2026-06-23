local sys = require "luci.sys"
local m = Map("zlan_mqtt", translate("MQTT"), translate("Publicação das tags Modbus e keepalive do sistema."))
local s = m:section(NamedSection, "main", "mqtt", translate("Broker"))
s.addremove = false

local o = s:option(Flag, "enabled", translate("Habilitar")); o.rmempty = false
o = s:option(Value, "broker", translate("Broker")); o.rmempty = false
o = s:option(Value, "port", translate("Porta")); o.default = "1883"; o.datatype = "port"
o = s:option(Value, "username", translate("Usuário"))
o = s:option(Value, "password", translate("Senha")); o.password = true
o = s:option(Value, "client_id", translate("Client ID")); o.rmempty = false
o = s:option(Value, "topic_prefix", translate("Prefixo")); o.rmempty = false
o = s:option(Value, "publish_interval", translate("Intervalo de publicação (s)")); o.default = "5"; o.datatype = "uinteger"
o = s:option(Value, "system_interval", translate("Keepalive do sistema (s)")); o.default = "600"; o.datatype = "uinteger"
o = s:option(ListValue, "qos", translate("QoS")); o:value("0"); o:value("1"); o:value("2")
o = s:option(Flag, "retain", translate("Retain")); o.rmempty = false

function m.on_after_commit()
    sys.call("/etc/init.d/zlan-telemetry restart >/dev/null 2>&1 &")
end

return m
