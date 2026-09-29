# Android

O OSInfotech pode ser empacotado como APK Android usando Fyne. O APK usa o
mesmo icone do programa e mantem um banco SQLite local no telefone.

## Pre-requisitos

- JDK 17 ou superior.
- Android SDK com `build-tools`, `platform-tools`, uma plataforma Android e
  Android NDK instalados.
- Variavel `ANDROID_HOME` apontando para o Android SDK.

## Gerar APK

```powershell
.\build-android.ps1 -Version 0.6.0 -Build 1
```

O arquivo sai em `dist\android\OSInfotech-Android-<versao>.apk`. O script
remove APKs anteriores dessa pasta antes de gerar a nova versao. O alvo padrao
e `android/arm64`, compativel com praticamente todos os celulares atuais. Para
outro alvo, informe por exemplo `-Target android/arm`.

## Atualizacao automatica (OTA por APK)

O Android e nativo (Go/Fyne), entao nao troca codigo em runtime como um bundle
JS. A atualizacao acontece baixando e instalando o APK novo por cima. O fluxo:

1. O build gera tambem `dist\android\latest.json` com
   `{ version, build, apk_url, apk_sha256, size_bytes, mandatory, notes }`.
2. Com `-Publish`, o APK e o manifesto sobem para o R2 em `android/`:

   ```powershell
   .\build-android.ps1 -Version 0.6.13 -Build 613 `
     -PublishBaseUrl "https://pub-edaac30d04ca45e1a0f8a19919ac915c.r2.dev" -Publish
   ```

   O `-Build` segue o esquema monotonico `maior*10000 + menor*100 + patch`
   (0.6.13 -> 613) para o versionCode nunca regredir.
3. No app, apos o login (e no menu `Conta` > `Atualizar app`), ele le
   `https://<base>/android/latest.json`, compara com a versao instalada
   (`Metadata().Version`) e, se houver versao nova, oferece baixar o APK. O
   download abre no navegador/gerenciador do Android; o usuario toca no arquivo
   e o sistema instala por cima.

O usuario precisa permitir "instalar apps desconhecidos" para o navegador na
primeira vez (limitacao do Android para sideload).

**Bootstrap:** o OTA so funciona a partir de uma versao que ja tenha o codigo
de checagem embutido (>= 0.6.13). A primeira instalacao dessa versao ainda e
manual; a partir dela os proximos updates viram automaticos.

## Limite atual

O banco SQLite e os logins desta versao sao locais a cada aparelho. Para que
desktop e celulares compartilhem os mesmos usuarios, OS, clientes e permissoes
em tempo real, sera necessario um backend de dados e autenticacao. No
Cloudflare, a arquitetura indicada e Worker + D1 para os dados e R2 somente
para anexos, avatares e PDFs.
