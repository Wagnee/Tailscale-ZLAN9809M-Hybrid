local sys = require "luci.sys"
local m = Map("zlan_modbus", translate("Modbus TCP"), translate("Dispositivos e tags persistentes."))
local diagnostics = m:section(SimpleSection)
diagnostics.template = "zlan_hybrid/modbus_status"
local s = m:section(TypedSection, "device", translate("Dispositivos"))
s.addremove = true
s.anonymous = false
s.template = "cbi/tblsection"

local o = s:option(Flag, "enabled", translate("Habilitar")); o.rmempty = false
o = s:option(Value, "name", translate("Nome"))
o = s:option(Value, "ip", translate("IP")); o.datatype = "ipaddr"
o = s:option(Value, "port", translate("Porta")); o.default = "502"; o.datatype = "port"
o = s:option(Value, "slave_id", translate("Slave ID")); o.default = "1"; o.datatype = "range(0,247)"
o = s:option(Value, "poll_interval", translate("Polling (s)")); o.default = "30"; o.datatype = "uinteger"
o = s:option(Value, "timeout", translate("Timeout (s)")); o.default = "5"; o.datatype = "uinteger"

s = m:section(TypedSection, "tag", translate("Tags"))
s.addremove = true
s.anonymous = false
s.template = "cbi/tblsection"
o = s:option(Flag, "enabled", translate("Habilitar")); o.rmempty = false
o = s:option(ListValue, "device", translate("Dispositivo"))
m.uci:foreach("zlan_modbus", "device", function(section)
    o:value(section[".name"], section.name or section[".name"])
end)
o = s:option(Value, "name", translate("Nome"))
o = s:option(Value, "address", translate("Endereço")); o.datatype = "uinteger"
o = s:option(ListValue, "type", translate("Tipo"))
o:value("holding", translate("Holding register")); o:value("input", translate("Input register"))
o:value("coil", translate("Coil")); o:value("discrete", translate("Discrete input"))
o = s:option(Value, "scale", translate("Escala")); o.default = "1"
o = s:option(Value, "offset", translate("Offset")); o.default = "0"

function m.on_after_commit()
    sys.call("/etc/init.d/zlan-telemetry restart >/dev/null 2>&1 &")
end

return m
