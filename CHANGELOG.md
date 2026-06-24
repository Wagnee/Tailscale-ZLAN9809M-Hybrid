# Changelog

## 0.2.0

- limita `tailscale up` a 30 segundos para evitar um processo bloqueado indefinidamente em `NeedsLogin`;
- captura e exibe a URL de autenticação manual do Tailscale;
- registra corretamente o comando `status` no `rc.common` do firmware;
- adiciona estado e logs diretamente às páginas Modbus e MQTT;
- adiciona publicação e subscrição MQTT para diagnóstico;
- adiciona escrita e confirmação manual de coil Modbus;
- registra indícios do OOM killer quando o `tailscaled` é encerrado.
- detecta automaticamente a rota LAN `/24` em instalações novas, sem sub-rede fixa incorreta.

## 0.1.0

- primeira versão híbrida para o hardware ZLAN9809M validado;
- Tailscale dinâmico em `/tmp`, com hash fixado e estado persistente;
- telemetria Modbus TCP e MQTT unificada;
- interface LuCI, diagnóstico, atualização, limpeza e remoção;
- instalação sem `opkg` e com validação de hardware.
