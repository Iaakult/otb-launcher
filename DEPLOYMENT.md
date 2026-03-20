# OTBaiak Launcher Deployment

Base URL usada pelo launcher:
- https://www.otbaiak.com/launcher/

## Estrutura esperada no servidor web

Crie os arquivos dentro de ./html/launcher com esta estrutura:

- ./html/launcher/OTBaiak.exe
- ./html/launcher/OTBaiak.exe.sha256
- ./html/launcher/tibia1511/client.windows.json
- ./html/launcher/tibia1511/client.linux.json
- ./html/launcher/tibia1511/client.mac.json
- ./html/launcher/tibia1511/assets.windows.json
- ./html/launcher/tibia1511/assets.linux.json
- ./html/launcher/tibia1511/assets.mac.json
- ./html/launcher/otclient/client.windows.json
- ./html/launcher/otclient/client.linux.json
- ./html/launcher/otclient/client.mac.json
- ./html/launcher/otclient/assets.windows.json
- ./html/launcher/otclient/assets.linux.json
- ./html/launcher/otclient/assets.mac.json

## Regras dos manifests

- Cada arquivo em "files" deve ter "url" relativa ao diretório do jogo.
- Exemplo para tibia1511:
  - "url": "bin/client.exe.lzma"
- Exemplo para otclient:
  - "url": "otclient"

## Fluxo no launcher

- Ao clicar em Play, o launcher checa update do jogo selecionado.
- Se houver arquivos faltando ou diferentes de hash, ele baixa tudo e abre o jogo.
- Se nao houver update, abre o jogo imediatamente.

## Atualizacao do proprio launcher

- O launcher compara o hash em OTBaiak.exe.sha256 com o executavel local.
- Quando o hash muda, baixa https://www.otbaiak.com/launcher/OTBaiak.exe e reinicia.
