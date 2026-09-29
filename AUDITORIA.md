# Auditoria técnica — DBS Assistência Técnica

Varredura completa feita em 23–24/09/2026 sobre o app Go/Fyne (20 mil linhas), o
worker Cloudflare (848 linhas) e o banco de produção sincronizado.

Cada item foi confirmado lendo o código, e os que dependiam de dado real foram
conferidos no banco de produção. Nada aqui é suposição.

Legenda: **[CORRIGIDO]** já está no código e com teste. **[DECISÃO SUA]** precisa
de uma escolha sua, de uma ação no Cloudflare ou de um deploy do worker.

---

## 1. Segurança: o mais grave

### 1.1 As chaves do R2 estão dentro de todo aplicativo distribuído — [DECISÃO SUA]

`cloud.go:21-22` guarda o Access Key ID e o Secret Key do R2 como constantes.
Constante em Go vira texto dentro do binário: conferi com `grep` no
`dist/portable/OSInfotech.exe` e a chave está lá, legível. O mesmo vale para o
APK publicado.

Quem tiver o aplicativo (qualquer técnico, qualquer pessoa que baixe o APK) tem
credencial de escrita no bucket inteiro. E o bucket é onde mora a atualização
automática: `latest.json` e o instalador. A cadeia completa:

1. Extrai a chave do APK.
2. Sobrescreve o `latest.json` e o instalador que ele aponta.
3. Todo PC da loja baixa e **executa** esse arquivo no próximo "Atualizar".

A verificação SHA-256 não protegia porque o hash vinha do mesmo manifesto
adulterado. Isso eu já fechei (item 1.3), mas a chave continua exposta.

O que fazer, em ordem:

1. Revogar o par de chaves no painel da Cloudflare **hoje**. Ele já circulou.
2. Parar de embarcar chave no app. Os envios de foto e PDF passam a ser feitos
   pelo worker, que já autentica o usuário, e o app só manda o arquivo para lá.
3. Enquanto isso não existe, uma chave nova apenas adia o problema: o próximo
   build volta a vazá-la.

### 1.2 Os termos de venda estão públicos e numerados em sequência — [DECISÃO SUA]

`term_upload.go:35` grava os PDFs como `terms/generated/TERMO-000001.pdf`, e o
bucket responde sem autenticação. Testei com requisição HEAD, sem baixar
conteúdo:

```
TERMO-000001.pdf -> HTTP 200
TERMO-000002.pdf -> HTTP 200
TERMO-000003.pdf -> HTTP 200
```

Qualquer pessoa que descubra o endereço do bucket (ele está dentro do app)
baixa todos os termos trocando o número. Os termos têm CPF, RG, endereço,
telefone e IMEI. É exposição de dado pessoal, com a LGPD em cima.

As fotos dos aparelhos estão no mesmo bucket, com nome derivado de hash do
conteúdo, o que é bem melhor. Já o avatar do cliente
(`avatar.go:296`) usa hash do **nome**: quem souber o nome de um cliente monta o
endereço da foto dele.

Saída: tirar o acesso público do bucket e servir os arquivos por link assinado
com validade, ou pelo próprio worker. Enquanto for público, qualquer termo novo
nasce exposto.

### 1.3 Atualização automática aceitava arquivo sem assinatura — [CORRIGIDO]

`updater.go` só conferia o SHA-256 quando o manifesto trazia o campo. Manifesto
sem hash, arquivo executado sem conferência. Agora a instalação é recusada sem a
assinatura.

### 1.4 A sessão salva no aparelho é falsificável — [DECISÃO SUA]

`session.go:75-78` grava a sessão como três valores num arquivo JSON comum
(`preferences.json`): um booleano, o número do usuário e a data de validade. Não
há segredo, assinatura nem conferência com o servidor.

Numa loja onde todo mundo usa o mesmo Windows, um técnico abre esse arquivo,
troca o número do usuário para 1 (o administrador é sempre o 1) e estica a data.
Na próxima abertura ele entra como administrador, sem senha: vê custo, relatório,
e pode excluir OS e clientes.

Saída possível: guardar um token assinado pelo servidor em vez do número do
usuário, ou exigir a senha de novo a cada X dias mesmo com "manter conectado".

### 1.5 O técnico pode apagar qualquer coisa pela API — [CORRIGIDO, no deploy 0.12.0]

O worker exige usuário autenticado em `/v1/sync/push`, mas não olha o papel
(`index.js:58-60` e `293-346`). A regra "só administrador exclui" existe apenas
nos botões do app. Com o próprio token, que está em texto claro no SQLite local,
dá para mandar uma exclusão de qualquer OS e ela some para todo mundo, ou
reescrever o custo de qualquer OS.

> **Corrigido em 0.12.0.** `applySyncChange` recusa exclusão de quem não é
> administrador, com a mensagem indo de volta na resposta daquela alteração.

### 1.6 Senha nova não é aplicada quando a nuvem está ligada — [CORRIGIDO, no deploy 0.12.0]

`auth_ui.go:281-309` envia só papel e situação; o worker (`index.js:227-259`)
também só lê papel e situação. O app mostra "Usuário salvo" e grava auditoria,
mas nem o hash local nem o da nuvem mudam. Ou seja: **não existe troca de senha
hoje**. Se a senha de um técnico vazar, ela continua valendo.

> **Corrigido em 0.12.0.** O app envia a senha nova, o worker troca o hash e
> **apaga as sessões abertas** daquele usuário, e o hash local também é trocado,
> que é o que vale quando a loja fica sem internet.

### 1.7 Usuário desativado ainda entra sem internet — [DECISÃO SUA]

`auth_ui.go:392-397`: quando a nuvem não responde, o app cai no login local, e a
linha local continua com `active=1` até aquele usuário fazer um login com nuvem.
Demitiu alguém? Ele desliga o Wi-Fi do notebook da loja e entra normalmente, por
tempo indeterminado.

### 1.8 Sem limite de tentativas de senha — [CORRIGIDO, no deploy 0.12.0]

`index.js:130-151` aceita tentativas ilimitadas, e cada uma roda 100 mil
iterações de PBKDF2 no seu worker. É ataque de adivinhação e conta de CPU ao
mesmo tempo.

> **Corrigido em 0.12.0.** Oito erros seguidos travam a conta por 15 minutos, e a
> conferência do bloqueio vem **antes** do PBKDF2.

### 1.9 A senha do administrador está em texto no LEIA-ME da prévia — [DECISÃO SUA]

`dist/portable/LEIA-ME-teste.txt` traz login e senha do administrador **de
produção**. Se essa pasta for compartilhada com alguém, é acesso total. Tire a
senha do arquivo e troque a senha assim que 1.6 estiver resolvido.

---

### 1.10 A senha do aparelho era impressa no comprovante do cliente — [CORRIGIDO em 0.14.0]

O campo "Anotações da bancada" diz ao operador, na própria tela, que **"não
aparece para o cliente"**. E aparecia: `escpos.go:55` imprimia `Obs.` no
comprovante térmico e `receipt.go:133` no PDF.

Foi exatamente onde a loja guardou a senha do aparelho. Na base de 24/09, de 94
OS com anotação preenchida, **51 falam em senha e 44 têm o código de desbloqueio
escrito**: `SENHA: 200508`, `TA COM CAPINHA SENHA: 155128`, `ORÇAR, SENHA 1307`.
Reimprimir qualquer uma dessas OS punha o código no papel que o cliente leva.

> **Corrigido em 0.14.0**, em quatro camadas. A anotação **saiu dos dois
> comprovantes**, o que fecha o vazamento de toda a base de uma vez, inclusive
> do que ainda não foi classificado. As 28 senhas inequívocas foram movidas para
> o campo próprio (`device_key`, que nunca é impresso). As 15 restantes viram
> tarja na lista de OS, para uma pessoa olhar. E digitar uma senha nas anotações
> passa a oferecer, na hora, mover para o campo certo.

---

### 1.11 A máscara do telefone inventava um número — [CORRIGIDO em 0.14.0]

`inputs.go` cortava os dígitos em 11 sem olhar onde eles estavam. Com dois
telefones no mesmo campo, o resultado juntava o fim do primeiro com o começo do
segundo e produzia um número bem formatado que não é de ninguém:

    "5198104717 ou  51997077640"   ->  (51) 98104-7175
    "51 995379117     E 997067628" ->  (51) 99537-9117 (era o certo por acaso)
    "51 5597252513"                ->  (51) 55972-5251

Não é recusar, é **fabricar**: o operador não tem como desconfiar. Com o
WhatsApp ligado desde a 0.11.0, mandaria a OS do cliente para um terceiro.

> **Corrigido em 0.14.0.** A separação entre os números está no texto e passou a
> ser respeitada: os grupos de dígitos são juntados enquanto couberem num
> telefone, e o que passa disso fica de fora, com aviso na tela. Na base, 23 das
> 220 OS não têm telefone que receba WhatsApp, e agora isso aparece no cadastro
> em vez de semanas depois.

---

## 2. Perda silenciosa de dados na sincronização

Esta é a parte que mais me preocupou: quando falha, ninguém percebe.

### 2.1 O técnico responsável era apagado a cada sincronização — [CORRIGIDO]

`sync_service.go` gravava `technician_id = 0, technician_name = ''` toda vez que
uma OS chegava da nuvem, mesmo com o dado vindo certo do servidor. Efeito em
cadeia: a OS ficava "Sem responsável definido", e quem abrisse e salvasse virava
o dono dela (`page_orders.go:719`), o filtro "Só minhas OS" perdia a OS, e a
regra que impede um técnico de mexer na OS de outro parava de valer.

Agora o app traduz o usuário da nuvem para o técnico local, e quando não conhece
esse usuário mantém o responsável que já estava. Dois testes cobrem os dois
casos.

### 2.2 OS excluída sumia para sempre do aparelho — [CORRIGIDO]

Ao excluir, o vínculo com a nuvem ficava para trás apontando para uma linha que
não existia mais. Se a nuvem recusasse a exclusão (outro aparelho editou a OS
antes), a OS continuava viva para todo mundo e **invisível** só para quem tentou
excluir, para sempre.

Agora: exclusão confirmada apaga o vínculo, e uma atualização que não encontra a
linha local recria a OS em vez de sumir com ela. Dois testes.

### 2.3 Edição feita depois de um envio que estourou o tempo era descartada — [CORRIGIDO]

A fila atualizava o conteúdo da alteração, mas mantinha a revisão antiga
(`sync_store.go:196-201`). Depois de um envio que expirou, a edição seguinte
nascia com revisão velha, era tratada como conflito e **a versão do usuário era
jogada fora**. Uma linha de SQL resolveu.

### 2.4 A leitura da nuvem podia girar para sempre — [CORRIGIDO]

O laço de leitura (`sync_service.go:168`) confiava no cursor devolvido pelo
servidor. Se ele pedisse "tem mais" sem avançar, o app girava eternamente: pílula
travada em "Sincronizando…" e sincronização morta pelo resto da sessão. Agora ele
para e avisa.

### 2.5 Duas sincronizações podiam rodar juntas — [CORRIGIDO]

A trava era solta em dois lugares, e como `fyne.Do` é assíncrono a segunda
liberação apagava a trava de uma sincronização que ainda estava rodando. Duas
rodadas simultâneas disputam a mesma fila e o mesmo cursor. Removi a liberação
duplicada (era código meu, de hoje).

### 2.6 Conflito sobrescreve a edição local sem avisar — [DECISÃO SUA]

`sync_service.go:141-147`: no conflito o app aplica a versão do servidor por cima
e apaga a alteração local da fila. Numa sincronização automática, que roda a cada
save, isso acontece **sem nenhum aviso na tela**.

Cenário real: dois aparelhos com a OS 142. Um muda o valor para R$ 500 e
sincroniza. O outro marca "Entregue" e sincroniza: a marcação some, o status
volta, ninguém é avisado. Precisa de uma decisão sua sobre o que mostrar: um
aviso, uma tela de "o que fazer", ou uma cópia da versão perdida.

### 2.7 O servidor não trava a revisão na hora de gravar — [CORRIGIDO, no deploy 0.12.0]

O worker lê a revisão, e só depois grava, sem transação e sem `AND revision = ?`
no UPDATE (`index.js:318-341`). Dois aparelhos que enviam no mesmo segundo passam
os dois pela conferência e os dois "ganham": uma das edições evapora e **nenhum
dos dois recebe aviso de conflito**. É o mesmo bug de 2.6, mas do lado do
servidor e sem nem detectar.

Junto disso: gravar a entidade e gravar o diário de alterações são duas chamadas
separadas. Se o worker morrer entre as duas, a nuvem fica com o dado novo e sem
registro no diário, e os outros aparelhos **nunca** recebem essa alteração.

Correção: `AND revision = ?` no UPDATE, tratar zero linhas como conflito, e as
duas gravações dentro de um `env.DB.batch()`.

> **Corrigido em 0.12.0.** Feito assim, com uma diferença: o
> registro no diário é um `INSERT ... WHERE EXISTS` condicionado à revisão nova,
> e o conflito é detectado relendo a revisão gravada, não pela contagem de
> linhas do driver.

### 2.8 Uma linha ruim trava a sincronização inteira — [CORRIGIDO, no deploy 0.12.0]

Uma OS sem cliente, sem aparelho ou sem defeito faz o worker devolver 400 para o
lote inteiro (`index.js:408-410`). O app não remove nada da fila e, como o envio
vem antes da leitura, **o aparelho também para de receber**. Para sempre, até
alguém mexer no banco.

Conferi no banco de produção: hoje não existe nenhuma linha assim. O risco é
latente, não ativo. A correção é isolar o erro por alteração, em vez de derrubar
o lote.

> **Corrigido em 0.12.0.** Nos dois lados. O worker isola por alteração; o app,
> que devolvia erro e parava tudo ao ver uma recusa, agora tira aquela alteração
> da fila, segue com as outras e mostra o motivo. A OS continua gravada aqui e
> volta para a fila assim que for corrigida.

### 2.9 A primeira carga cresce sem limite — [DECISÃO SUA, precisa de deploy]

A leitura faz uma consulta por alteração, em série (`index.js:277-289`), e o
diário nunca é compactado: uma OS editada 80 vezes é lida e reaplicada 80 vezes.
O custo da primeira sincronização de um aparelho novo cresce com o total
histórico de edições, não com a quantidade de registros. Já foi preciso baixar a
página de 100 para 50 por causa disso. Continua piorando sozinho.

---

## 3. Dinheiro e relatórios

### 3.1 O relatório usa a data da última edição como data de entrega — [CORRIGIDO em 0.12.0]

`store.go:894` filtra por `updated_at`. Não existe coluna de data de entrega. Uma
OS entregue em julho e corrigida em setembro **sai do relatório de julho e entra
no de setembro**, e o PDF imprime "entregue em setembro".

Isso significa que um relatório exportado em julho e o mesmo relatório exportado
hoje não batem, e nada no app explica a diferença.

Piorei isso hoje sem querer: a união automática de clientes repetidos carimbava
`updated_at` em todas as OS do cliente unido. **Já corrigi** essa parte (item
3.2), mas a causa raiz continua: falta uma coluna `delivered_at`, gravada quando
o status vira "Entregue" e enviada para a nuvem. É mudança de schema e de worker.

> **Corrigido em 0.12.0.** A coluna existe, é carimbada pelo `Save` (e não por quem
> chama, que são quatro caminhos diferentes), e o relatório filtra por ela. As OS
> entregues antes disso herdaram `updated_at` uma única vez, o que congela a
> atribuição que já estava valendo.

### 3.2 Renomear ou unir cliente não muda mais a data das OS — [CORRIGIDO]

`customers.go:253` carimbava a data atual em toda OS do cliente. Com a união
automática rodando a cada sincronização, isso reescreveria datas de entrega o
tempo todo. Agora a data da OS não é tocada.

### 3.3 Colar um valor sem centavos divide por 100 — [DECISÃO SUA]

`inputs.go:88` trata o campo como maquininha de cartão: os dígitos entram pela
direita. Digitar é perfeito. **Colar** "1.234" vira R$ 12,34, e "R$ 250" vira
R$ 2,50.

Não mexi porque o comportamento de digitação é intencional e a escolha é sua.
Uma saída é: se o texto colado tiver separador com três dígitos depois (1.234),
tratar como reais inteiros. Digitar dígito a dígito continua igual.

### 3.4 O técnico via o valor bruto com o título "Lucro líquido" — [CORRIGIDO]

Para quem não é administrador o bloco mostrava o valor cobrado sob o título de
lucro. Agora o título muda para "Valor cobrado" e a legenda explica.

### 3.5 Recibo em PDF 80 mm corta o que passa de 280 mm — [DECISÃO SUA]

`thermal.go:77` desliga a quebra de página com altura fixa. Um laudo longo
(~2.700 caracteres somando defeito, observações e acessórios) faz o PDF terminar
no meio, **sem o valor do serviço e sem a linha de assinatura**. A impressão
direta na bobina não tem esse problema.

### 3.6 Data gravada em formato que não ordena direito — [CORRIGIDO EM PARTE em 0.12.0]

`store.go:1527` usa RFC3339Nano, que corta zeros à direita. Comparando como
texto, `...T03:00:00.4372Z` vem **antes** de `...T03:00:00Z`. Na prática o
relatório erra por menos de um segundo nas bordas do período: uma OS salva em
00:00:00.4 do primeiro dia fica de fora. É pequeno, mas é errado, e também bagunça
a ordenação dentro do mesmo segundo.

> **Corrigido em 0.12.0.** As colunas novas (`delivered_at`, `paid_at`) usam
> largura fixa com precisão de segundo, e é por elas que o relatório compara
> período agora. `created_at` e `updated_at` **ficaram como estavam de
> propósito**: `updated_at` é o cursor da sincronização, e reescrever a coluna
> inteira reenviaria a base toda para a nuvem.

---

## 4. Erros de comportamento no dia a dia

### 4.1 Foto podia entrar na OS errada — [CORRIGIDO]

O envio da foto demora; se o técnico abrisse outra OS no meio, a foto era anexada
na OS aberta no momento em que o envio terminava — foto do aparelho de um cliente
virando prova de entrada de outro. Agora cada envio lembra de qual OS saiu.

### 4.2 Status fora da lista gravava o status da OS anterior — [CORRIGIDO]

O componente de seleção do Fyne ignora em silêncio um valor que não está na
lista (confirmei no código do Fyne 2.7.4). Abrir uma OS com status antigo deixava
na tela o status da OS anterior, e salvar gravava esse valor errado. Vale para OS
e para termos. Hoje não há nenhuma OS assim no banco; agora também não pode
acontecer.

### 4.3 Telefone colado com +55 era cortado — [CORRIGIDO]

`inputs.go` cortava nos 11 primeiros dígitos: colar "5551998765432" gravava
"(55) 51998-7654", um telefone que não existe. E telefone agora é identidade de
cliente, então isso também criava cliente repetido. Agora o 55 do país é
removido.

### 4.4 Excluir termo não exigia administrador — [CORRIGIDO]

Era a única exclusão sem a conferência de papel; só o botão desabilitado
protegia.

### 4.5 Campo de custo nascia visível para o técnico — [CORRIGIDO]

A página da OS é montada na primeira abertura, depois do login, e a regra de
papel só era aplicada no login seguinte. Nenhum valor vazava (o campo vem vazio e
é ignorado ao salvar), mas o campo aparecia.

### 4.6 Foto de um cliente podia pintar no cadastro de outro — [CORRIGIDO]

Duas aberturas seguidas, e a resposta mais lenta sobrescrevia a mais nova. Agora
vale sempre a última.

### 4.7 Foto que falhou nunca mais carregava — [CORRIGIDO]

Se a internet piscasse, a imagem ficava marcada como carregada para sempre e só
voltava reabrindo o app.

### 4.8 A senha era lida de dentro da goroutine — [CORRIGIDO]

Continuar digitando durante o login podia mandar a senha pela metade.

### 4.9 Formulário aberto sobrescreve o que a sincronização trouxe — [DECISÃO SUA]

`store.go:545` grava o formulário inteiro sem conferir se a linha mudou no meio
do caminho. Com a OS aberta na tela, uma sincronização traz a versão nova; o
técnico digita uma palavra em observações e salva, e o valor e o status voltam
para o que estavam. Precisa de uma escolha: recarregar a tela quando o registro
aberto muda, ou avisar antes de salvar.

### 4.10 Técnico enxerga a OS de todos — [DECISÃO SUA]

`store.go:793` não filtra por técnico; só existe a caixinha "Só minhas OS",
desligada por padrão. Editar já é bloqueado, ver não. E o banco local de cada
aparelho tem o custo de todas as OS, sem senha: abrir o arquivo com qualquer
visualizador de SQLite mostra tudo.

---

## 5. Qualidade técnica

- **Teste com detector de corrida falha** (`go test -race`): `TestMobilePagesFitViewport`.
  A corrida é do driver de teste do Fyne, que executa `fyne.Do` na mesma
  goroutine, e não acontece com o app de verdade. Mas trava o uso de `-race`.
- **Sem transação em operações de vários passos**: unir e renomear cliente, e
  salvar OS, fazem várias gravações seguidas. Uma falha no meio deixa estado
  quebrado. Com SQLite e uma conexão só, é barato embrulhar em transação.
- **Sem trilha de auditoria local para OS**: a tabela `audit_events` do app só
  registra usuários. Quem excluiu uma OS, ou quem mudou o status, não está
  registrado no aparelho. A nuvem registra o autor de cada alteração
  (`index.js:691`), mas não há tela para consultar.
- **Sem encerramento controlado**: nenhuma goroutine é cancelada ao fechar a
  janela. Um envio de foto em andamento sobe o arquivo para o R2 e morre antes de
  gravar a referência: arquivo órfão no bucket, cobrado, sem uso.
- **`%` e `_` digitados na busca viram curinga de SQL.** Buscar "100%" traz
  qualquer coisa que comece com 100.
- **Sem retenção**: `sync_changes`, `sessions` e `audit_events` só crescem.

---

## 6. O que eu sugiro fazer, na ordem

1. **Hoje**: revogar as chaves do R2 e tirar a senha do LEIA-ME.
2. **Esta semana**: fechar o bucket para leitura pública (item 1.2). É o que tem
   dado pessoal de cliente exposto agora.
3. **Próximo deploy do worker**, num pacote só: travar a revisão no UPDATE com
   `batch()` (2.7), isolar o erro por alteração (2.8), exigir administrador para
   excluir (1.5), permitir troca de senha (1.6) e limitar tentativas de login
   (1.8).
4. **Depois**: coluna `delivered_at` (3.1), envio de arquivo pelo worker no lugar
   da chave embarcada (1.1), aviso de conflito na tela (2.6).
5. **Quando der**: os itens de comportamento do dia a dia (4.9, 4.10, 3.3, 3.5).

O que já está corrigido está no código com teste, e o app continua compilando e
com a suíte inteira passando.
