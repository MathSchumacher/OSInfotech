-- Fechamento da OS e campos que ate agora ficavam presos num computador so.
--
-- delivered_at existe porque o relatorio usava a data da ultima edicao como
-- data de entrega: corrigir um telefone numa OS de julho movia aquela receita
-- para o mes da correcao. payment_method e paid_at respondem se a loja recebeu,
-- que "Entregue" sozinho nao dizia.
--
-- device_key (senha do aparelho) e contacted_at (data da cobranca por WhatsApp)
-- ja existiam no aplicativo desde a 0.9.0, mas nao tinham coluna aqui, entao
-- nunca saiam do computador onde foram digitados.

ALTER TABLE service_orders ADD COLUMN device_key TEXT NOT NULL DEFAULT '';
ALTER TABLE service_orders ADD COLUMN delivered_at TEXT NOT NULL DEFAULT '';
ALTER TABLE service_orders ADD COLUMN paid_at TEXT NOT NULL DEFAULT '';
ALTER TABLE service_orders ADD COLUMN payment_method TEXT NOT NULL DEFAULT '';
ALTER TABLE service_orders ADD COLUMN contacted_at TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_service_orders_delivered
    ON service_orders(organization_id, delivered_at);

-- Tentativas de login por conta. Sem isto cada chute rodava 100 mil iteracoes
-- de PBKDF2 no worker: adivinhacao de senha e conta de CPU ao mesmo tempo.
CREATE TABLE IF NOT EXISTS login_attempts (
    login_name TEXT PRIMARY KEY,
    attempts INTEGER NOT NULL DEFAULT 0,
    first_attempt_at TEXT NOT NULL,
    blocked_until TEXT
);
