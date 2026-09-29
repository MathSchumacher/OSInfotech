# OSInfotech API

Worker privado que centraliza usuarios e dados do OSInfotech. O D1 configurado
em `wrangler.toml` e `a64f0547-1048-462d-a025-80c8f2617764`.

## Deploy inicial

1. No terminal, entre nesta pasta e autentique o Wrangler:

   ```powershell
   npx wrangler login
   ```

   Alternativamente, use `CLOUDFLARE_API_TOKEN`. O token precisa ter, no
   escopo da conta, `D1:Edit`, `Workers Scripts:Edit` e `R2:Edit`. As permissoes
   `User Details:Read` e `Memberships:Read` evitam avisos do Wrangler.

2. Aplique a migracao no D1 remoto:

   ```powershell
   npx wrangler d1 migrations apply osinfotech --remote --config wrangler.toml
   ```

3. Crie um token forte para o primeiro administrador e registre-o apenas como
   secret do Worker:

   ```powershell
   npx wrangler secret put SETUP_TOKEN --config wrangler.toml
   ```

4. Publique:

   ```powershell
   npx wrangler deploy --config wrangler.toml
   ```

O Worker exibira uma URL `workers.dev`. Depois, associe um dominio como
`api.seudominio.com` no painel Cloudflare.

## Rotas iniciais

- `GET /health`
- `GET /v1/setup/status`: informa se a organizacao ja foi ativada.
- `POST /v1/setup`: cria a organizacao e primeiro administrador. Exige o
  header `X-Setup-Token` e so funciona enquanto nao houver usuarios.
- `POST /v1/auth/login`
- `POST /v1/auth/logout`
- `GET /v1/me`
- `GET /v1/organization`: dados da empresa ativa na nuvem.
- `PATCH /v1/organization`: administrador; altera o nome da empresa.
- `GET /v1/users`: administrador.
- `POST /v1/users`: administrador; cria somente tecnicos.
- `PATCH /v1/users/:id`: administrador; promove/rebaixa ou ativa/desativa um tecnico.
- `POST /v1/sync/push`: recebe alteracoes locais de clientes, OS e termos.
- `GET /v1/sync/pull?after=<cursor>`: entrega alteracoes incrementais da organizacao.

As acoes de criacao e promocao ficam registradas em `audit_events`. O primeiro
administrador e criado exclusivamente por `POST /v1/setup`; nenhum aparelho
novo deve exibir esse setup depois que a organizacao ja foi inicializada.

O desktop e o APK mantem SQLite somente como cache offline. O D1 e a fonte
compartilhada: cada alteracao local entra em fila, e a API aplica revisoes para
evitar sobrescrever uma versao mais nova do outro dispositivo. Nenhuma chave
S3/R2 deve ser enviada para o cliente: o Worker tem o binding privado `FILES`
para o bucket `osinfotech`.
