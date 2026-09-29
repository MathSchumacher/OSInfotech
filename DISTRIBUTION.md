# Distribuicao Windows

O projeto gera duas versoes:

- `dist/portable/DBS-assistencia.exe`: executavel portable.
- `dist/setup/DBS-assistencia-Setup-<versao>.exe`: instalador por usuario.
- `dist/latest.json`: manifesto para atualizacao automatica.

## Gerar tudo

```powershell
.\build.ps1 -Version 0.1.0
```

Com URL publica final, por exemplo Cloudflare R2:

```powershell
.\build.ps1 -Version 0.3.0 -PublishBaseUrl "https://cdn.seudominio.com/osinfotech" -Publish
```

## Gerar somente portable

```powershell
.\build.ps1 -Version 0.1.0 -SkipSetup
```

## O que o setup faz

- Instala em `%LOCALAPPDATA%\Programs\OSInfotech`.
- Cria atalho no Menu Iniciar e na Area de Trabalho.
- Registra desinstalacao em `Adicionar ou remover programas` via HKCU.
- Abre o app ao fim da instalacao, exceto se executado com `/silent`.

O banco de dados continua em `%APPDATA%\OSInfotech`, separado da pasta do app.

## Atualizacao via Cloudflare R2

1. Crie `.r2.env.local` a partir de `.r2.env.example`.
2. Gere e publique com:
   `.\build.ps1 -Version x.y.z -PublishBaseUrl "<public-url>" -Publish`.
3. No app instalado, abra `Atualizacoes` e informe a URL publica do
   `latest.json`.

O app compara `version`, baixa `setup_url`, valida `setup_sha256` quando
informado e inicia o instalador.

O publicador calcula o uso total do bucket antes de subir. Se o upload faria o
R2 passar de 10 GB, ele remove releases antigos (`DBS-assistencia-Setup-*` e
`DBS-assistencia-Portable-*`) ate abrir espaco. Ele nunca apaga `latest.json` nem
avatars de cliente.

## Avatars de cliente

No app, use a aba `Clientes` para criar/editar perfis sem abrir uma OS. Ao
escolher uma foto no perfil do cliente, o app:

- faz crop central quadrado;
- redimensiona para 512x512;
- converte para WebP;
- sobe em `avatars/<hash>.webp`;
- salva a URL no perfil local do cliente.

Se o bucket passaria de 10 GB, o upload do avatar e bloqueado.

## Nome e icone

O executavel se chama `DBS-assistencia.exe` e o instalador coloca o app em
`%LOCALAPPDATA%\Programs\DBS Assistencia Tecnica`, com atalho e entrada em
Programas e Recursos com esse nome.

Duas coisas de proposito **nao** mudaram de nome, porque mudar apagaria dados de
quem ja usa o app:

- a pasta de dados, que continua em `%APPDATA%\OSInfotech` (banco local);
- o identificador do app, `br.com.osinfotech.desktop`, que e a chave das
  preferencias (login lembrado, sessao mantida, tema).

O instalador novo remove sozinho a instalacao antiga chamada OSInfotech: mata o
processo, apaga os atalhos, a entrada de desinstalacao e a pasta em
`%LOCALAPPDATA%\Programs\OSInfotech`. Sem isso o cliente ficaria com dois apps.

O icone sai do proprio logo, gerado por `python tools/make_icon.py`: o recorte
fechado no controle nos tamanhos pequenos (16 a 64 px) e o emblema completo nos
grandes (128 e 256). Depois de trocar o logo ou o icone, recompile o recurso do
Windows antes do `go build`:

```powershell
windres -O coff -F pe-x86-64 -i app.rc -o app_windows.syso
windres -O coff -F pe-x86-64 -i cmd/installer/app.rc -o cmd/installer/app_windows.syso
```

O `build.ps1` ja faz isso sozinho.
