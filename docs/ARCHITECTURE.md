# Arquitetura

## Divisão entre flash e RAM

```text
Flash /overlay (persistente)          RAM /tmp (apagada no reboot)
--------------------------------     --------------------------------
/usr/bin/zlan-tailscale-loader  ---> /tmp/tailscale.combined
/usr/bin/zlan-telemetryd              /tmp/tailscale, /tmp/tailscaled
/etc/init.d/zlan-*                    /tmp/tailscale-runtime/*.sock
/etc/config/zlan_*                    /tmp/zlan-telemetry/*.json
/etc/tailscale/tailscaled.state       logs limitados
LuCI e ferramentas de manutenção
```

Somente o executável Tailscale é dinâmico. Configurações, identidade, telemetria, interface e serviços continuam disponíveis sem internet.

## Loader Tailscale

O serviço `zlan-tailscale` mantém um processo shell pequeno. O loader:

1. aguarda internet sem bloquear o boot;
2. baixa para um arquivo `.part`;
3. exige mais de 7 MB e confere o SHA-256 configurado;
4. testa o executável antes de promovê-lo;
5. inicia `tailscaled` com socket e state file explícitos;
6. executa `tailscale up` apenas com opções permitidas pela configuração e timeout limitado;
7. monitora o processo e repete a operação em caso de falha.

Uma queda de internet não apaga o estado nem reinicia continuamente um daemon saudável.

Quando não existe auth key nem identidade válida, o `tailscale up --json` fornece uma URL de autenticação. O loader grava essa URL em `/tmp/zlan-tailscale-auth-url`, exibe-a no status e encerra o cliente após 30 segundos, mantendo somente o daemon. Isso evita que duas instâncias grandes do executável permaneçam na RAM indefinidamente.

## Telemetria

Modbus e MQTT foram reunidos em um único executável Go estático para eliminar as bibliotecas dinâmicas ausentes no firmware e reduzir o uso da flash. Cada dispositivo Modbus possui seu próprio intervalo de polling. O documento de estado é atualizado atomicamente em `/tmp` e o publicador envia somente leituras novas com qualidade `good`.

As configurações são UCI, portanto podem ser alteradas pelo LuCI ou pela linha de comando. O daemon não grava medições na flash.

O mesmo executável possui comandos de diagnóstico de curta duração para publicar/subscrever MQTT e escrever/confirmar um coil Modbus. O controller LuCI aceita apenas parâmetros validados, usa ações POST protegidas por token e não executa comandos fornecidos diretamente pelo usuário.

## Migração

O instalador interrompe e remove somente caminhos conhecidos dos projetos anteriores. Ele preserva:

- `/etc/tailscale/tailscaled.state`;
- qualquer `/etc/config/zlan_*` já existente;
- todas as configurações não relacionadas ao projeto.

O binário antigo em `/usr/sbin` é removido para recuperar a flash antes da cópia do novo payload. O projeto não executa remoção genérica de pacotes.

## Atualizações

Não há atualização automática de código remoto. `zlan-hybrid-update` precisa ser chamado explicitamente, baixa o instalador oficial via HTTPS e reutiliza a validação de hardware e do pacote. Os arquivos UCI e a identidade são mantidos.
