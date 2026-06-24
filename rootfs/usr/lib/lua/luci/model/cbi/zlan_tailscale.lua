local sys = require "luci.sys"
local m = Map("zlan_tailscale", translate("Tailscale dinâmico"),
    translate("O binário fica em /tmp e é baixado somente quando há internet. Estado e configuração permanecem na flash."))
local diagnostics = m:section(SimpleSection)
diagnostics.template = "zlan_hybrid/tailscale_status"
local s = m:section(NamedSection, "main", "main", translate("Configuração"))
s.addremove = false

local o = s:option(Flag, "enabled", translate("Habilitar"))
o.rmempty = false
o = s:option(Value, "hostname", translate("Hostname Tailscale"))
o.rmempty = false
o = s:option(Value, "routes", translate("Rotas anunciadas"))
o.placeholder = "192.168.9.0/24"
o = s:option(Flag, "accept_dns", translate("Aceitar DNS"))
o.rmempty = false
o = s:option(Flag, "accept_routes", translate("Aceitar rotas"))
o.rmempty = false
o = s:option(Value, "netfilter_mode", translate("Modo netfilter"))
o:value("nodivert", "nodivert")
o:value("on", "on")
o:value("off", "off")
o = s:option(Value, "auth_key", translate("Auth key"))
o.password = true
o.rmempty = true
o = s:option(Value, "binary_url", translate("URL do tailscale.combined"))
o.rmempty = false
o = s:option(Value, "binary_sha256", translate("SHA-256 esperado"))
o.rmempty = false
o = s:option(Value, "up_timeout", translate("Timeout do login (s)"))
o.default = "30"
o.datatype = "range(10,300)"

function m.on_after_commit()
    sys.call("/etc/init.d/zlan-tailscale restart >/dev/null 2>&1 &")
end

return m
