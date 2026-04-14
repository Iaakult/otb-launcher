# OTBaiak Launcher Deployment

Base URL usada pelo launcher:
- https://www.otbaiak.com/launcher/

## Estrutura esperada no servidor web

Crie os arquivos dentro de ./html/launcher com esta estrutura:

- ./html/launcher/OTBaiak-Launcher.exe
- ./html/launcher/OTBaiak-Launcher.exe.sha256
- ./html/launcher/version.json
- ./html/launcher/otclient.zip

## Formato do version.json

```json
{
  "otclient": {
    "version": "1.0",
    "executable": "OTBaiak OTC.exe"
  }
}
```

## Regras do pacote

- O launcher baixa apenas `otclient.zip`.
- O ZIP deve conter os arquivos finais prontos para rodar.
- Nao use `.lzma` dentro do ZIP.
- Nao use manifests de assets.
- Nao use patch incremental.
- O launcher preserva apenas a pasta `characterdata` durante update.

## Cliente ativo

- O unico cliente mantido pelo launcher e o `otclient`.
- Na interface do launcher, ele aparece como `OTBaiak Client`.

## Fluxo no launcher

- Ao clicar em Play, o launcher checa `version.json`.
- Se a versao remota for diferente da local, baixa `gameID.zip`.
- Remove a pasta antiga, preservando apenas `characterdata`.
- Extrai o ZIP diretamente no diretorio do jogo.
- Se nao houver update, abre o jogo imediatamente.

## Atualizacao do proprio launcher

- O launcher compara o hash do executavel publicado no servidor.
- Quando o hash muda, baixa o novo launcher e reinicia.
