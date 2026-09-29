# OSInfotech

> **Redesign 2026-09 — DBS Assistência Técnica.** A interface foi refeita (tema escuro/claro, seções por funcionalidade, busca global, design system). Veja `DESIGN.md`. O identificador técnico do app continua `osinfotech`.


App desktop em Go para criar e consultar ordens de servico de assistencia tecnica.

## Recursos

- Banco SQLite local com WAL para boa resposta em uso diario.
- Cadastro de OS com CPF, nome, telefone, dispositivo, marca/modelo, serie/IMEI, motivo, acessorios, observacoes e valor em R$.
- Login local para administrador e tecnicos, com senha protegida por hash e responsavel registrado em cada OS.
- Valor bruto, custo interno e lucro liquido por OS; o consolidado faturado considera OS entregues e fica restrito ao administrador.
- Lista pesquisavel e filtros por `Pendente`, `Aprovada` e `Entregue`.
- Aba `Clientes` com lista, busca, cadastro, edicao e exclusao de perfis.
- Edicao, exclusao e contadores por status.
- Geracao de comprovante HTML imprimivel.
- Icone proprio aplicado no executavel, janela do app, atalhos e comprovantes.
- Autopreenchimento de cliente pelo historico: ao digitar um nome ja usado, o app sugere o perfil e preenche CPF/telefone.
- Atualizacoes via `latest.json` hospedado em HTTP/S, compativel com Cloudflare R2.
- Publicacao autonoma para Cloudflare R2 via S3 API.
- Foto de perfil de cliente com crop central, conversao para WebP e upload para R2.
- R2 ja vem pre-configurado no binario desta distribuicao.
- PDF de OS para impressora termica de 80 mm, acessivel pelo botao `80 mm` na OS.
- Script de geracao de APK Android em `build-android.ps1`; veja `MOBILE.md` para os requisitos.

## Rodar

```powershell
go run .
```

Para compilar do fonte no Windows, mantenha um GCC/MSYS2 disponivel no `PATH`
porque a interface desktop e o SQLite usam CGO.

## Compilar

```powershell
.\build.ps1 -Version 0.1.0
```

Isso gera o portable em `dist\portable\OSInfotech.exe`, um ZIP portable e o
instalador em `dist\setup`.

Para gerar links absolutos no manifesto de atualizacao e publicar no R2:

```powershell
.\build.ps1 -Version 0.3.0 -PublishBaseUrl "https://cdn.seudominio.com/osinfotech" -Publish
```

O app distribuido ja vem com R2 pre-configurado. Para o publicador local, tambem
e possivel usar `.r2.env.local` ou variaveis de ambiente:

```text
R2_ACCOUNT_ID=
R2_BUCKET_NAME=
R2_ENDPOINT=
R2_PUBLIC_BASE_URL=
R2_ACCESS_KEY_ID=
R2_SECRET_ACCESS_KEY=
```

Use `.r2.env.example` como modelo. O arquivo `.r2.env.local` e ignorado pelo Git.

No app, `Nuvem R2` permite revisar/testar a configuracao e `Atualizacoes`
permite verificar o manifesto. O limite padrao do bucket e 10 GB; o publicador
remove releases antigos quando necessario, e o app bloqueia upload de avatar se
o limite seria ultrapassado.

O banco fica em:

```text
%APPDATA%\OSInfotech\osinfotech.db
```

## Sincronizacao desktop e Android

O projeto inclui o Worker Cloudflare e a migracao do D1 em
`cloudflare/worker`. O D1 sera a fonte compartilhada de usuarios, OS, clientes
e termos; o SQLite local passara a atuar como cache offline. O R2 continua
reservado para arquivos, avatares, PDFs e releases.

Veja `cloudflare/worker/README.md` para aplicar a migracao e publicar a API.

Os comprovantes ficam em:

```text
%APPDATA%\OSInfotech\recibos
```
