# Hardware e firmware validados

Este projeto foi dimensionado a partir da sonda de hardware executada no equipamento real, sem alterar a configuração do dispositivo.

## Resultado da sonda

- modelo reportado: `ZLAN zlan-cat1 (16M flash)`;
- SoC: MediaTek MT7628AN, revisão 1, eco 2;
- CPU: MIPS 24KEc, 580 MHz, MIPS32r1/r2, um núcleo;
- firmware: OpenWrt 21.02.0 r16279;
- target: `ramips/mt76x8`;
- arquitetura de pacotes: `mipsel_24kc`;
- kernel: 5.4.143 com ABI customizado `fe24b4bf9114ead2296e0f8cacdd593a`;
- RAM: 123.060 KB reportados pelo kernel;
- flash: SPI NOR de 16 MB;
- overlay JFFS2: 6.080 KB, com 5.280 KB livres na medição;
- `/tmp`: tmpfs de 61.528 KB, com aproximadamente 53.008 KB livres;
- `/dev/net/tun`: existente, com o módulo TUN já carregado;
- `ca-bundle`: já instalado;
- ferramentas observadas: BusyBox `sh`, `awk`, `sed`, `grep`, `wget` e UCI;
- requisitos adicionais verificados pelo instalador antes de alterar a flash: `tar` e `sha256sum`;
- ferramentas ausentes: `bash`, `curl`, `xz` e `upx`.

## Consequências técnicas

O pacote offline anterior expandia um `tailscaled.xz` para cerca de 23 MB dentro de `/usr/sbin`. Isso não cabe no overlay de 6 MB. Mesmo o arquivo compactado de aproximadamente 5 MB consumiria praticamente toda a flash gravável.

O módulo `kmod-tun` oficial também não deve ser instalado. O hash do ABI encontrado no feed oficial (`81b5...`) não corresponde ao kernel do fabricante (`fe24...`). O TUN funcional já faz parte do firmware.

Os feeds configurados no equipamento incluem:

- uma origem `openwrt_core` apontando incorretamente para `https://openwrt.org`;
- o feed inexistente `lora` para OpenWrt 21.02;
- declarações duplicadas em versões anteriores da configuração.

Por isso o instalador não lê, altera ou atualiza os feeds e nunca usa `opkg`.

## Limites operacionais

O `tailscale.combined` tem 7.781.256 bytes em disco e é executado a partir de `/tmp`. Como é um executável UPX, sua imagem descompactada é maior na RAM. O loader exige memória disponível antes de iniciar e mantém o daemon de telemetria desligado enquanto Modbus e MQTT não forem configurados.

O controle de frequência da CPU é condicional. Esse kernel pode não expor `/sys/devices/system/cpu/cpu0/cpufreq`; nesse caso o projeto apenas informa que o recurso não é suportado.
