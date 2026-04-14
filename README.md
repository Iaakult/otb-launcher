# OTBaiak Launcher

Este launcher usa um fluxo simples de atualizacao por ZIP.

## Fluxo de update

1. Baixa `version.json` do servidor.
2. Compara a versao remota com `.launcher_version` local.
3. Se houver update, baixa `otclient.zip`.
4. Remove a instalacao anterior por completo, preservando apenas `characterdata`.
5. Extrai o ZIP diretamente, sem conversao, sem patch incremental e sem compressao adicional.
6. Salva a nova versao local.

## Regras do pacote

- O launcher baixa apenas arquivos `.zip` padrao.
- O conteudo extraido fica exatamente como esta dentro do ZIP.
- O ZIP precisa conter os executaveis e DLLs finais prontos para uso.
- O launcher nao usa `.lzma`.
- O launcher nao gera, nao interpreta e nao depende de `client.json`, `assets.json` ou manifests de patch.

## Arquivos esperados no servidor

- `version.json`
- `otclient.zip`
- `OTBaiak-Launcher.exe`
- `OTBaiak-Launcher.exe.sha256`

## Cliente publicado

- O launcher distribui apenas o cliente `otclient`.
- Na interface, esse cliente aparece como `OTBaiak Client`.

## Observacao operacional

Se o ZIP enviado ao servidor contiver arquivos compactados adicionalmente, como `.lzma`, eles serao extraidos exatamente assim. O servidor deve publicar um ZIP final e executavel.
