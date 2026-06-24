# Tailscale ZLAN9809M Hybrid

Projeto específico para o gateway **ZLAN9809M** com firmware OpenWrt 21.02 customizado. Ele mantém na flash apenas os componentes pequenos e persistentes. O `tailscale.combined`, que já foi validado nesse equipamento, é baixado sob demanda para `/tmp` quando a internet fica disponível.

O instalador não executa `opkg update` nem instala pacotes. Isso é intencional: os feeds presentes no firmware contêm URLs inválidas e o ABI do kernel não coincide com os módulos oficiais do OpenWrt 21.02.

## Hardware suportado

| Item | Hardware validado |
|---|---|
| Equipamento | ZLAN9809M, variante CAT1 com flash de 16 MB |
| SoC / CPU | MediaTek MT7628AN, MIPS 24KEc, 580 MHz, 1 núcleo |
| Firmware | OpenWrt 21.02.0, `ramips/mt76x8`, `mipsel_24kc` |
| Kernel | 5.4.143 customizado |
| RAM | 128 MB |
| Overlay | JFFS2 de aproximadamente 6 MB |
| TUN | Deve estar integrado ao firmware e disponível em `/dev/net/tun` |

O instalador recusa outro hardware, RAM insuficiente, ausência de TUN ou menos de 2,5 MB livres no overlay.

## O que está incluído

- carregamento dinâmico e recuperação automática do Tailscale;
- estado/identidade Tailscale persistente em `/etc/tailscale`;
- anúncio de rota da LAN, hostname, auth key, DNS e rotas configuráveis por UCI/LuCI;
- polling Modbus TCP com coils, discrete inputs, holding e input registers;
- escala e offset por tag Modbus;
- publicação MQTT das tags alteradas e keepalive do sistema;
- interface LuCI para Tailscale, Modbus, MQTT, estado, logs, RAM e armazenamento;
- publicação/subscrição MQTT de teste e escrita manual confirmada de coil Modbus pelo LuCI;
- monitor de CPU/temperatura e governor somente quando suportado pelo kernel;
- atualização manual controlada, diagnóstico, limpeza segura e desinstalação.

O terminal web externo do projeto antigo não é incluído: ele nunca fazia parte do código-fonte, dependia de um pacote remoto e aumenta a superfície de ataque. A limpeza agressiva de pacotes e o executor automático de scripts remotos também foram substituídos por operações limitadas ao próprio projeto.

## Instalação

No SSH do ZLAN9809M:

```sh
wget -O /tmp/install-zlan-hybrid.sh \
  https://raw.githubusercontent.com/Wagnee/Tailscale-ZLAN9809M-Hybrid/main/install.sh
sh /tmp/install-zlan-hybrid.sh
```

O instalador valida o equipamento, baixa o pacote persistente com SHA-256, migra artefatos conhecidos das versões anteriores, preserva o estado Tailscale e inicia os serviços. Ele pode solicitar hostname, rota LAN e auth key quando executado em um terminal interativo.

Depois, use:

```sh
zlan-hybrid-status
logread -e zlan-tailscale
tail -f /tmp/zlan-tailscale.log
```

Se o estado mostrar `NeedsLogin`, configure uma auth key em **Serviços > ZLAN Hybrid > Tailscale** ou abra a URL exibida por `zlan-hybrid-status`. O comando de login possui timeout de 30 segundos para não manter uma segunda instância pesada do executável bloqueada indefinidamente. Em instalações novas com LAN `/24`, o instalador detecta a rota pelo UCI em vez de assumir uma sub-rede fixa.

No LuCI, acesse **Serviços > ZLAN Hybrid**.

## Funcionamento do Tailscale

1. O loader persistente inicia no boot e lê `/etc/config/zlan_tailscale`.
2. Quando há internet, ele baixa `assets/tailscale.combined` para `/tmp/tailscale.combined.part`.
3. Confere tamanho, SHA-256 e execução antes da troca atômica para `/tmp/tailscale.combined`.
4. Cria os links temporários `tailscale` e `tailscaled`, inicia o daemon e aplica as opções UCI.
5. Se o processo ou a conexão falhar, tenta novamente sem escrever o binário na flash.

Após um reboot, apenas o binário temporário precisa ser baixado novamente. A identidade permanece em `/etc/tailscale/tailscaled.state`, evitando novo cadastro.

## Modbus e MQTT

Os arquivos persistentes são:

- `/etc/config/zlan_modbus`
- `/etc/config/zlan_mqtt`

Exemplo de tag:

```uci
config device 'plc1'
    option enabled '1'
    option name 'PLC Principal'
    option ip '192.168.9.100'
    option port '502'
    option slave_id '1'
    option poll_interval '30'
    option timeout '5'

config tag 'temperatura'
    option enabled '1'
    option device 'plc1'
    option name 'Temperatura'
    option address '40001'
    option type 'holding'
    option scale '0.1'
    option offset '0'
```

São aceitos endereços absolutos (`40001`, `30001`, `10001`, `1`) ou offsets baseados em zero. As publicações usam os tópicos:

```text
<prefixo>/<dispositivo>/<tag>
<prefixo>/system/keepalive
```

O daemon de telemetria só inicia quando MQTT ou ao menos uma seção Modbus está habilitada, economizando RAM no uso exclusivo do Tailscale.

As páginas **Modbus** e **MQTT** mostram o estado do serviço, o estado de runtime e os últimos logs. A página MQTT permite publicar e aguardar uma mensagem de teste. A página Modbus permite escrever um coil real e confirmar o valor por leitura; use essa operação somente em uma saída cujo acionamento seja seguro.

## Atualização e remoção

Atualização explícita, preservando configurações e identidade:

```sh
zlan-hybrid-update
```

Remoção preservando configurações e identidade:

```sh
zlan-hybrid-uninstall
```

Remoção total, incluindo a identidade Tailscale:

```sh
zlan-hybrid-uninstall --purge
```

## Desenvolvimento

Pré-requisitos no Windows: Go 1.22+ e UPX. Defina `GO_EXE` e `UPX_EXE` se eles não estiverem no `PATH`:

```powershell
$env:GO_EXE = "C:\caminho\go.exe"
$env:UPX_EXE = "C:\caminho\upx.exe"
.\build.ps1
```

O build executa os testes, compila estaticamente com `GOOS=linux`, `GOARCH=mipsle`, `GOMIPS=softfloat`, comprime o daemon e recusa um pacote persistente maior que 2,5 MB.

Detalhes adicionais estão em [docs/HARDWARE.md](docs/HARDWARE.md) e [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Segurança

- O `tailscale.combined` possui SHA-256 fixado na configuração padrão.
- O pacote persistente também é validado por SHA-256 antes da instalação.
- Auth key e senha MQTT ficam em arquivos UCI com modo `0600`.
- A interface LuCI usa ações POST autenticadas e não aceita comandos arbitrários.
- Não existe daemon que baixe e execute scripts automaticamente.

## Licença

Apache-2.0. O binário Tailscale incluído continua sujeito às licenças e avisos do projeto Tailscale.
