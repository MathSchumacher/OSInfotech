-- Contato alternativo e pagamento parcial.
--
-- contact_name/contact_phone existem porque o campo do telefone virou campo de
-- recado: 11 OS da base escrevem quem atende dentro dele ("51 99917-0194 -
-- NANDO", "51999373754 ESPOSA", "MANDA MENSAGEM PELO DA MAE"). Hoje isso nem e
-- mais possivel digitar, porque a mascara apaga letras, entao a loja perdeu uma
-- capacidade que usava. E 23 das 220 OS nao tem telefone que receba WhatsApp:
-- um segundo caminho ate o cliente e o que destrava a cobranca dessas.
--
-- paid_cents existe porque o modelo de pagamento da 0.12.0 e binario, e a loja
-- trabalha com sinal: "100 de entrada", "foi pago 100 dos 220 da tela e
-- conector, falta 150". O que falta receber passa a ser a diferenca, e nao o
-- valor cheio.

ALTER TABLE service_orders ADD COLUMN contact_name TEXT NOT NULL DEFAULT '';
ALTER TABLE service_orders ADD COLUMN contact_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE service_orders ADD COLUMN paid_cents INTEGER NOT NULL DEFAULT 0;

-- Quem ja tinha pagamento registrado pagou o valor cheio: era o unico estado
-- que o modelo binario sabia representar.
UPDATE service_orders SET paid_cents = gross_amount_cents
WHERE paid_at <> '' AND paid_cents = 0 AND gross_amount_cents > 0;
