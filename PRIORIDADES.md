# Revisão de prioridades — DBS Assistência Técnica

Levantamento feito em 24/09/2026 sobre o banco de produção sincronizado (219 OS,
208 clientes, 3 meses de uso) e sobre o código. Os números abaixo foram medidos,
não estimados. As recomendações são propostas: o que é decisão de negócio está
marcado como tal.

## Situação em 24/09/2026: o Lote 1 está feito

Tudo que está marcado abaixo já está no código, com teste, e entrou na versão
0.9.0:

| Feito | O que mudou |
|---|---|
| Cobrança por WhatsApp | Botão dentro da OS e na lista, com a mensagem pronta contendo número da OS, aparelho e orçamento. A data da cobrança fica registrada. |
| Idade da OS | A lista mostra "parada há 12 dias" em cor de alerta, e o sino conta só o que passou de 7 dias em vez de contar tudo. |
| Status sem abrir a OS | Tocar no status na lista abre o menu com os outros status e a cobrança. Antes eram 5 cliques por OS. |
| Reabrir a mesma OS | Clicar de novo na linha que acabou de ser aberta funciona. |
| Teclado | Tab anda em todos os campos e Ctrl+S, Ctrl+N e Ctrl+K funcionam com o cursor dentro do texto. |
| Ordem do foco | Valor e custo saíram do fim do trilho para o meio do formulário, alcançáveis pelo teclado. |
| Salvar e nova OS | Botão próprio, e imprimir agora salva sozinho antes. |
| Impressora lembrada | A escolhida da última vez vem selecionada. |
| Sugestões | Defeito, marca/modelo e tipo de aparelho oferecem o que a loja já escreveu, por frequência, sem levar junto a senha que estava no meio do texto. |
| Senha do aparelho | Campo próprio, fora do comprovante do cliente. |
| Garantia no recibo | Rodapé com prazo de garantia e de retirada, editável em Configurações. |
| Cadastro pela metade | Sair do editor com texto digitado pergunta antes de descartar. |
| Busca | Espera o operador parar de digitar em vez de varrer a base a cada tecla. |
| Modelos de checklist | Param de duplicar a cada aparelho novo, e os repetidos são unidos. |
| Celular | Botão Voltar do Android não fecha mais o app, toque tem retorno visual, campos numéricos abrem teclado numérico, e o script de build calcula o versionCode sozinho. |

O que continua aberto está nos lotes 2 a 5, no fim deste documento. O lote 2, que
depende de um deploy do worker, é o próximo passo natural: sem ele não há data de
entrega, pagamento nem sincronização dos campos novos entre aparelhos.

## Se você só ler isto

A plataforma virou um excelente caderno de entrada e para aí. Registra 2,5
aparelhos por dia com telefone e defeito em 100% dos casos, e depois **42% das OS
ficam paradas em "aguardando aprovação"**, sendo metade sem nunca ter recebido um
orçamento. São R$ 12.469 esperando um "pode fazer", e 76 desses clientes têm
WhatsApp utilizável no cadastro.

A prioridade número um não é velocidade nem tela nova: é **fazer a OS andar** e
**fechar o dinheiro**. Velocidade eu medi e tem anos de folga. O cadastro tem
ganhos grandes e baratos, principalmente destravar o teclado. E o celular tem
defeitos que impedem o uso.

---

## 1. O que a plataforma é hoje, medido

| | |
|---|---|
| OS registradas | 219 em 3 meses |
| Ritmo | 54 em julho, 85 em agosto, 80 nos primeiros 24 dias de setembro |
| Média | cerca de 2,5 OS por dia, crescendo |
| Clientes | 208, sendo que 188 apareceram uma vez só |
| Aparelho | 165 de 219 são celular (75%) |
| Contas de usuário | 1 (admin) |
| Entregue no período | 96 OS, R$ 15.860, ticket médio R$ 165 |
| Dias até entregar | 29 no mesmo dia, 24 em até 3 dias, 33 em até 8, 10 acima disso |

O uso é de balcão: alguém registra o aparelho na entrada, com o celular do
cliente sempre preenchido (219 de 219). O app cumpre bem esse papel.

## 2. O diagnóstico central: entra tudo, não anda nada

**91 das 219 OS estão paradas em "Aguardando aprovação". É 42% de tudo que já
passou pela loja.** Abrindo esse número:

| | |
|---|---|
| Nunca foram editadas depois de criadas | 63 de 91 |
| Sem nenhum valor lançado, ou seja, nunca orçadas | 45 |
| Com orçamento feito, esperando resposta do cliente | 46, somando R$ 12.469 |
| Paradas há mais de 30 dias | 23 |

Metade dos aparelhos que entram nunca recebe um orçamento, e a outra metade
recebe e ninguém volta a falar com o cliente. São R$ 12.469 parados esperando um
"pode fazer".

Uma ressalva honesta: parte dessas 91 pode já ter sido resolvida no mundo real,
com o cliente levando o aparelho embora sem conserto, e ninguém atualizou a tela.
A loja sabe fechar OS quando quer, tanto que usa "Não autorizado" 8 vezes e "Sem
conserto" 17. De qualquer forma a conclusão é a mesma: **o status não reflete a
realidade**, e é ele que alimenta o painel, o sino e o relatório.

O app tem o telefone de **todas** as 219 OS. E tem botão de WhatsApp. Só que o
botão está na tela do cliente ([page_customers.go:203](page_customers.go#L203)),
não na OS e não na lista. Para cobrar uma aprovação hoje o caminho é: abrir a
lista, filtrar por status, abrir a OS para ver o valor, ir em Clientes, buscar o
nome, abrir o cadastro, clicar em WhatsApp e digitar a mensagem do zero. São oito
passos, e o link nem leva texto pronto ([page_customers.go:244](page_customers.go#L244)).
Multiplique por 46 e está explicado por que 63 OS nunca foram tocadas de novo.

Dá para alcançar 76 dessas 91: são as que têm telefone em formato utilizável.
As outras 15 têm número incompleto e ficaram inalcançáveis. No total da base, 184
dos 219 telefones servem para abrir o WhatsApp, e 35 não. Como o telefone é o
único canal com o cliente, vale avisar na hora do cadastro quando o número está
incompleto, em vez de descobrir semanas depois que não dá para cobrar.

O sino do topo mostra 91 e nunca abaixa. Um alerta que marca 91 há semanas parou
de ser alerta e virou enfeite ([page_home.go:380](page_home.go#L380)).

### O que eu proponho

1. **Ação de cobrança em um toque, onde o problema aparece.** Botão de WhatsApp
   na linha da lista e dentro da OS, com mensagem pronta: número da OS, aparelho,
   defeito e valor orçado. O `wa.me` aceita texto pré-preenchido e hoje isso não é
   usado.
2. **Registrar o contato.** Um campo "cobrado em" com data. Sem isso não dá para
   saber se o cliente foi avisado uma vez ou nenhuma, e o operador não sabe de
   onde continuar.
3. **Separar "esperando orçamento" de "esperando o cliente".** — FEITO em 0.13.0.
   Os dois casos eram o mesmo status e são problemas opostos. Medido de novo em
   24/09: **46 dependem da loja** (falta dar o preço, a mais antiga parada há 56
   dias) e **46 dependem do cliente** (R$ 12.469,10 orçados e parados). Resolvido
   **sem status novo**: a distinção é derivada do valor lançado, que já está
   gravado desde sempre, então não houve migração, coluna nova nem deploy — e
   quando o operador lança o preço, a OS troca de fila sozinha.
4. **Envelhecimento visível.** A OS parada há 40 dias precisa parecer diferente da
   que entrou ontem, na própria lista. E o sino deve contar o que está atrasado,
   não o que existe.

## 3. O dinheiro não fecha — FEITO em 0.12.0

| | |
|---|---|
| Entregues sem custo lançado | 63 de 96 |
| Entregues sem valor lançado | 15 de 96 |
| Custo total lançado | R$ 4.210 contra R$ 15.860 faturados |

O painel mostra "Lucro líquido" com destaque, mas o custo existe em 34% das OS
entregues. O número está errado na maior parte do tempo, e errado para mais.

Além disso a OS não tem **campo de pagamento**. Entregue não quer dizer pago, e
hoje não há como saber quem ficou devendo. Também não há **data de entrega**: o
relatório usa a data da última edição, então editar uma OS antiga move a receita
de mês (isso está detalhado no [AUDITORIA.md](AUDITORIA.md), item 3.1).

### O que eu proponho

1. **Pedir o custo na hora de marcar como entregue**, não antes. Um campo só,
   com o teclado já no lugar certo. Enquanto o custo for opcional no meio do
   formulário, ele não vai ser preenchido.
2. **Campo de pagamento na OS**: pago, forma e data. É o que falta para o
   relatório virar caixa de verdade.
3. **Coluna `delivered_at`**, gravada quando o status vira Entregue. Sem ela o
   relatório financeiro nunca vai fechar.
4. **Parar de chamar de lucro** o que é bruto menos um custo que quase nunca
   existe, até o custo ser capturado.

### O que foi feito

Os quatro itens acima, mais o que dependia do mesmo deploy.

1. **A pergunta saiu do formulário e foi para a entrega.** Marcar a OS como
   entregue abre uma caixa com duas perguntas: quanto custou para a loja e como
   o cliente pagou (Dinheiro, Pix, Débito, Crédito, Ficou devendo). Enquanto o
   custo foi um campo opcional no cadastro de entrada, quando a peça nem tinha
   sido comprada, ele não era preenchido — 63 de 96 OS entregues provam isso, e
   em setembro foram 20 de 20.
2. **Coluna `delivered_at`**, carimbada pelo `Save` e não por quem chama, porque
   são quatro caminhos que gravam OS. O relatório filtra por ela. As 96 OS
   antigas herdaram `updated_at` uma vez só, o que congela a atribuição que já
   estava valendo em vez de deixá-la andando.
3. **Pagamento na OS**: forma e data. "Ficou devendo" é o único estado que conta
   como dívida — as OS antigas, sem forma de pagamento registrada, contam como
   *registro faltando*, e não como cobrança a fazer, que seria inventar dívida.
4. **O painel parou de mentir.** Com custo faltando no período, o ladrilho deixa
   de se chamar "Lucro líquido" e vira "Resultado parcial · Não é lucro: falta
   custo"; o de custos diz em quantas OS o custo foi lançado; e o PDF, o Excel e
   o Word saem com o mesmo título honesto. Na OS sozinha, sem custo lançado o
   número grande é "Valor cobrado", não lucro.
5. **Ladrilho "A receber"** no relatório, e a lista mostra "devendo R$ x" na OS
   entregue sem pagamento.

Ficou de fora de propósito: **colar** valor sem centavos (item 3.3 do
[AUDITORIA.md](AUDITORIA.md)) continua dividindo por 100, porque o
comportamento de digitação é intencional e a escolha é sua.

## 4. O formulário pede o que ninguém preenche e não oferece o que todos digitam

Preenchimento real das 219 OS:

| Campo | Preenchido |
|---|---|
| Telefone | 219 |
| Defeito | 219 |
| Valor | 137 |
| Observações | 94 |
| CPF | 50 |
| Custo | 42 |
| Acessórios | 15 |
| Checklist de entrada | 3 |
| Fotos | 3 |
| IMEI / número de série | 2 |
| Técnico responsável | 0 |

E do outro lado, o que é digitado à mão repetidas vezes:

- "troca de tela" aparece 25 vezes, "tela" 10, "vírus" 7, "não carrega" 6.
- 66 das 219 OS têm um defeito que já tinha sido escrito igual antes. Com as
  variações de grafia ("troca de tela", "troca tela", "tela quebrada") o número
  real é maior.
- 62 das 219 repetem uma combinação de aparelho e modelo já registrada.
- O campo "tipo de aparelho" é texto livre e já tem 32 valores distintos, com
  marca entrando no lugar do tipo ("motorola" como tipo de aparelho).
- 25 OS estão sem marca e modelo.

**E tem um campo que falta, usado em 41% das OS: a senha do aparelho.** A palavra
"senha" aparece em 89 das 219 OS, 39 vezes dentro do campo de defeito e 50 dentro
das observações. O técnico precisa desbloquear o aparelho para testar o conserto,
não existe campo para isso, então a senha entra no meio do texto: "TROCA DE TELA
/ SENHA: 6873", "tela quebrada / senha T desen". Duas consequências:

- o campo de defeito é **impresso no recibo** que fica no balcão e vai junto do
  aparelho ([escpos.go:53](escpos.go#L53)), então a senha sai no papel;
- a senha é indexada na busca, sincronizada para a nuvem e aparece nas listas.

Um campo próprio de "senha ou desenho de desbloqueio", fora do recibo do cliente,
resolve os dois problemas e arruma o texto do defeito de quebra.

### O que eu proponho

1. **Sugestão a partir do histórico** nos dois campos mais longos, defeito e
   marca/modelo. Corta cerca de um terço da digitação e, de quebra, padroniza o
   texto, o que faz a busca funcionar melhor.
2. **Tipo de aparelho vira lista** com os casos reais (celular, TV, notebook,
   console, tablet, caixa de som) e opção de escrever outro. Resolve o "motorola
   como tipo" e deixa o relatório por tipo de aparelho possível.
3. **Recolher o que ninguém usa.** IMEI, acessórios e CPF saem do caminho
   principal e ficam atrás de um "mais detalhes". Nada é removido, só deixa de
   ocupar o fluxo do dia a dia.
4. **Valores frequentes em um toque.** Os 5 preços mais repetidos cobrem 52 das
   137 OS com valor. Botões de atalho no campo de valor economizam digitação e
   erro de vírgula.

## 5. O que fazer com checklist e fotos

Foram construídos e não são usados: 3 OS com checklist e 3 com foto, de 219.

Vale decidir, e é **decisão sua**: ou o recurso entra no fluxo de entrada de
verdade, ou some da tela principal. Do jeito que está, ocupa espaço e não protege
ninguém. A favor de insistir: foto de entrada é o que resolve discussão de "já
estava arranhado". Se for insistir, o caminho é a câmera direto na criação da OS,
no celular, e não um botão dentro de uma aba que ninguém abre. Contra: se o
atendimento é no balcão com desktop, tirar foto exige pegar o celular, e isso
nunca vai acontecer com pressa.

## 6. Depois da entrega não existe nada

O recibo lista cliente, equipamento, motivo, técnico e valor
([escpos.go:42-68](escpos.go#L42)). Não diz **prazo de garantia**, nem prazo para
retirada do aparelho, nem aviso sobre backup de dados. Para uma assistência
técnica isso é o que evita discussão no balcão.

E os retornos existem. Procurando o mesmo cliente com o mesmo tipo de aparelho
voltando em até 90 dias, aparecem 12 casos. A maioria é aparelho duplo no mesmo
dia, mas dois são retorno de serviço claro:

- MOTO G8 POWER, "troca de tela e conector" e 63 dias depois "troca de tela";
- MOTO G54, "troca de tela" e 77 dias depois "troca de tela".

Com 25 trocas de tela no período e ticket médio de R$ 165, refazer um serviço sem
saber que é garantia é prejuízo direto. O app tem os dados para avisar, e não
avisa: bastaria marcar o prazo na OS e destacar, na entrada, que aquele cliente e
aquele aparelho já passaram por aqui dentro do prazo.

## 7. O teclado não fecha o cadastro, e isso custa em toda OS

Esta parte eu conferi dentro do Fyne, não é suposição.

- **Tab morre no campo "Defeito relatado".** No Fyne, um campo de várias linhas
  captura o Tab e insere uma tabulação em vez de pular para o próximo campo. O
  formulário tem três campos assim, e o Defeito é obrigatório. Ou seja, **toda OS
  bate nessa parede**, e o operador precisa voltar para o mouse.
- **Ctrl+S, Ctrl+N e Ctrl+K não funcionam enquanto o cursor está num campo de
  texto.** O Fyne entrega o atalho primeiro para o widget focado, e o campo de
  texto descarta em silêncio o que não conhece. Os três únicos atalhos do app
  ficam inertes exatamente durante o preenchimento, que é quando serviriam.
- **Valor cobrado, custo e status estão depois de tudo na ordem do Tab**, atrás do
  checklist inteiro, porque o painel lateral vem depois da coluna principal. Como
  o campo de anotações trava o Tab logo antes, o valor é **inalcançável pelo
  teclado**: sempre exige atravessar a tela com o mouse. No celular a ordem já
  está certa, então isso é acidente de montagem no desktop, não escolha.
- **Enter não confirma nada no formulário.** Escolher um cliente na lista de
  sugestões é só com mouse: as linhas da sugestão não recebem foco.
- **Não existe "salvar e nova OS" nem "salvar e imprimir".** Depois de salvar, o
  app fica parado na OS; para começar a próxima são dois cliques.
- **A impressora não é lembrada.** Toda impressão reabre o diálogo e escolhe a
  padrão do Windows de novo. São 3 cliques por recibo.
- **Meio cadastro digitado se perde sem aviso.** Não há rascunho, não há aviso ao
  sair e não há desfazer. Qualquer clique na barra lateral com o cliente na frente
  do balcão apaga tudo. A função que reabriria a OS em edição existe no código e
  nunca é chamada por ninguém.
- **Mudar um status custa 4 cliques, mais 1 para voltar**, e não existe ação em
  massa nem troca direto na lista. Pior: ao voltar para a lista a linha continua
  selecionada, e o Fyne ignora um clique numa linha já selecionada, então clicar
  de novo na mesma OS **não faz nada** até clicar em outra antes.

E o que o app já sabe e mesmo assim é redigitado: o aparelho e o modelo do
cliente que volta aparecem só como texto apagado de sugestão, nunca como valor; o
IMEI não é guardado por cliente; acessórios e defeito não têm reaproveitamento
nenhum; e o modelo de checklist tem um tipo de aparelho associado que nunca é
usado para escolher a lista sozinho.

### O que eu proponho, em ordem de retorno pelo esforço

1. **Destravar o Tab** nos campos de várias linhas e **fazer os atalhos
   funcionarem dentro dos campos**. São correções pequenas e valem para todo o
   app.
2. **Mover valor e status para antes do checklist** na ordem de foco, ou para a
   coluna principal.
3. **"Salvar e nova OS"** e **"salvar e imprimir"**, com a impressora lembrada.
4. **Sugestão a partir do histórico** em defeito e marca/modelo.
5. **Aviso ao sair com cadastro pela metade.**
6. **Trocar status direto na lista**, sem abrir a OS.

## 8. O celular está quebrado em pontos que impedem o uso

O APK publicado ainda é o 0.7.1 e a reforma visual nunca foi compilada para
Android. Auditando o código para celular, a lista abaixo é o que mais dói para o
técnico. Marquei o que é certeza de código e o que precisa de aparelho.

**Já corrigido hoje.** O menu da conta era ancorado na área de conteúdo inteira e
não havia limite de tela, então no celular a folha era desenhada **acima do topo**:
sair da conta, sincronizar e trocar o tema ficavam inalcançáveis, sem outro
caminho. Era código meu desta semana. Corrigi e deixei um teste que falha se a
folha sair da tela de novo.

**Já corrigido junto com o Lote 1:** o botão Voltar do Android agora volta uma
tela em vez de fechar o app, o toque tem retorno visual em botões e listas, e os
campos de CPF, telefone e valor abrem o teclado numérico.

**Confirmado no código, ainda aberto:**

- ~~O botão Voltar do Android fecha o app.~~ Corrigido. O projeto nunca trata a tecla, então
  o Fyne encerra a activity. Apertar Voltar no meio de uma OS **perde o
  formulário inteiro**, incluindo as fotos já enviadas. Vai acontecer várias vezes
  por dia.
- **Nenhum toque tem retorno visual.** Os nove componentes interativos do design
  system reagem a mouse (hover e clique), e o Fyne no celular nunca chama esses
  eventos. O técnico toca em "Salvar OS" e não vê nada acontecer, o que produz
  toque duplo.
- **O fluxo de foto, que é a razão de existir o celular, está caro:** abre o
  seletor de arquivos do Android, não a câmera; aceita **uma foto por vez**,
  mesmo o código pedindo várias; cada envio faz uma **listagem do bucket inteiro**
  antes de subir o arquivo; e cada envio redesenha a grade baixando **todas** as
  fotos de novo em tamanho cheio. Cinco fotos custam quinze downloads. Se o envio
  falha, a foto some: a OS tem fila offline, a foto não tem.
- **Dois dos três botões do cabeçalho da OS no celular não funcionam.**
  "Imprimir 80 mm" e "Comprovante" geram o arquivo numa pasta privada do app e
  tentam abrir por `file://`, que o Android não resolve. Ocupam cerca de 100
  pontos de altura em toda OS.
- **Alvos de toque menores que 44 pontos**: o sino (40), as setas "ver todas" (30),
  os chips de período em Relatórios (30) e as linhas do menu (36).
- **Faltam atalhos no celular**: Relatórios não está na barra inferior, o filtro
  "Só minhas OS" existe mas nunca é colocado na tela, e o indicador de nuvem não
  aparece.
- **Todo campo numérico abre o teclado alfabético**, porque o app não informa o
  tipo de teclado. CPF, telefone, valor e IMEI, todos.
- **Três armadilhas de publicação**: o script usa `-Build 1` por padrão, e o
  Android recusa instalar sobre o 701 atual; a versão do utilitário do Fyne não é
  realmente fixada, e uma diferente muda a assinatura e impede a atualização; e o
  APK nunca confere o SHA-256, que no desktop é obrigatório.

**Precisa de aparelho para medir:** a reforma visual desenha painéis pixel a
pixel em Go, na thread de desenho, e o teclado do Android redimensiona a tela a
cada campo tocado, o que redesenha tudo de novo. Pode ficar lento no celular de
entrada. É o primeiro teste a fazer.

### O que eu proponho

1. Tratar o botão Voltar: sair da OS em vez de fechar o app, e avisar se houver
   cadastro pela metade.
2. Retorno visual no toque em todos os componentes.
3. Câmera direto na OS, várias fotos por vez, fila offline e cache em disco.
4. Esconder impressão e comprovante no celular, ou fazê-los compartilhar o
   arquivo em vez de abrir caminho privado.
5. Teclado numérico nos campos numéricos, alvos de toque em 44 e os atalhos que
   faltam.
6. Arrumar as armadilhas do script antes de qualquer publicação Android.

## 9. Velocidade: medida, e não é aqui que dói

Tempo por chamada no banco real de 219 OS, no computador de desenvolvimento:

| Operação | Tempo |
|---|---|
| Contadores do topo | 0,07 ms |
| Relatório financeiro do mês | 0,13 ms |
| Lista completa de OS | 0,9 ms |
| Lista de clientes | 1,5 ms |
| Busca de OS por nome | 3,1 ms |
| Busca de cliente | 2,6 ms |
| Busca global do Ctrl+K | 6,1 ms |

Nada disso é perceptível. E com a base crescendo:

| Tamanho da base | Busca de OS | Lista completa |
|---|---|---|
| 219 OS (hoje) | 3,3 ms | 0,8 ms |
| 1.752 OS (cerca de 18 meses) | 23 ms | 6,5 ms |
| 14.016 OS (cerca de 12 anos) | 183 ms | 54 ms |

Ou seja: o banco local aguenta anos sem otimização nenhuma. **Não vale investir
em velocidade de busca agora.**

Onde a lentidão existe de verdade é na **sincronização**: o servidor responde
cerca de 250 ms por alteração, uma consulta por item, e o diário de alterações
nunca é compactado. O custo da primeira carga de um aparelho novo cresce com o
total histórico de edições, não com a quantidade de registros. Já foi preciso
baixar a página de 100 para 50 por causa disso. Está detalhado no
[AUDITORIA.md](AUDITORIA.md), item 2.9, e depende de um deploy do worker.

## 10. Defeitos confirmados que vale corrigir no caminho

- **Os modelos de checklist estão duplicados no banco.** São 8 registros para 4
  modelos: 4 criados em 03/08 e 4 em 04/08, com identificadores diferentes na
  nuvem. Um segundo aparelho semeou a própria cópia dos padrões e sincronizou.
  Cada aparelho novo que instalar o app vai somar mais 4 para todo mundo. É a
  mesma família do problema de clientes repetidos que já corrigimos: dado padrão
  sem identidade estável entre aparelhos.
- **As 219 OS estão sem técnico responsável.** A causa era a sincronização, que
  zerava o campo ao aplicar a OS vinda da nuvem. Isso já foi corrigido hoje. A
  nuvem ainda tem o autor de cada alteração, então dá para recuperar a
  atribuição com uma releitura completa, se valer a pena.
- O restante está no [AUDITORIA.md](AUDITORIA.md), que continua valendo, com dois
  itens de segurança ainda abertos e que não dependem de código.

## 11. Catálogo de modelos e memória de preço — FEITO em 0.10.0

Testei a ideia do catálogo público de modelos contra o seu banco real antes de
opinar. Ela é boa, e o motivo principal não é o que parece.

### O que o catálogo resolve

O campo de modelo é texto livre, e a base mostra o estrago:

| Marca | OS | Formas diferentes de escrever |
|---|---|---|
| Samsung | 62 | 34 |
| Motorola | 31 | 23 |
| iPhone | 23 | 14 |
| Xiaomi/Redmi | 15 | 11 |
| Sem modelo nenhum | 25 | — |

"samsung", "samsung a14", "a14" e "samsung a16" são o mesmo tipo de informação
escrita de quatro jeitos. Além disso 26 OS têm só a marca, sem modelo.

### A lista pública existe e cabe no app

Baixei a lista de dispositivos que o Google publica (`supported_devices.csv`,
4,7 MB, 54 mil linhas). Filtrando para as marcas que a sua loja realmente
atende e somando os iPhones à mão, sobram **5.434 modelos em 104 KB de texto**.
Isso vai embutido no app: sem internet, sem servidor, sem custo.

Testei o casamento contra o que a loja digitou nos últimos três meses:
**73% das OS com modelo preenchido encontram o modelo no catálogo.** Os 27% que
não encontram são justamente o que não é celular: PS4, PS5, Xbox, TVs, AOC,
Philco, uma Oster. Ou seja, o catálogo ajuda os 75% de celular e **não pode
atrapalhar o resto**: continuar digitando livre tem que seguir funcionando.

### A busca fora de ordem que você descreveu funciona

É o mesmo motor que já usamos na busca de OS: quebra em termos, cada termo casa
por prefixo em qualquer parte do nome, e vale a interseção. Rodando de verdade:

| Digitado | Resultado |
|---|---|
| `13 iph` | iPhone 13, iPhone 13 Pro, iPhone 13 mini |
| `iph 13` | o mesmo |
| `a14` | Galaxy A14, Galaxy A14 5G |
| `g54` | moto g54 5G |
| `a03 core` | Galaxy A03 Core |
| `sam a1` | Galaxy A10, A11, A12 |

Falta só uma ordenação que favoreça as marcas que a loja atende, senão um
Infinix aparece na frente de um Redmi em "note 11".

### O que o catálogo desbloqueia, que vale mais que ele

Esta é a parte que me convenceu. Hoje a troca de tela aparece em 67 OS com preço
entre **R$ 80 e R$ 999**, média R$ 320. A diferença é o modelo, e a loja não
consegue aprender o próprio preço: sem normalizar, **só 2 combinações de
modelo+defeito se repetem em toda a base**.

Normalizando modelo e serviço, o mesmo banco passa a ter **9 combinações
repetidas cobrindo 24 OS**, com três meses de uso apenas:

| Modelo | Serviço | Cobrado |
|---|---|---|
| moto g22 | troca de tela | R$ 180 / 230 / 250 / 280 |
| Galaxy A14 | troca de tela | R$ 250 / 250 / 350 |
| moto g54 5G | troca de tela | R$ 280 / 280 / 380 |
| Galaxy A02 | troca de tela | R$ 180 / 200 |
| qualquer | formatar / vírus | R$ 60 / 60 / 60 |

Duas leituras. A primeira: dá para oferecer o preço da última vez no momento do
orçamento, e isso ataca direto as 45 OS que nunca foram orçadas. A segunda, mais
incômoda: **a mesma troca de tela no mesmo aparelho foi cobrada 180 numa vez e
280 noutra**. Não é diferença de peça, é falta de memória. Cada erro desses é
R$ 100 saindo pela porta.

E como o custo entra na mesma tela, é a chance de capturar o custo que hoje falta
em 63 das 96 OS entregues.

O melhor: **isso não precisa de tabela nova nem de servidor**. O preço sai de uma
consulta ao próprio histórico, filtrando por modelo canônico e serviço.

### Sobre a foto do modelo

Aqui eu recomendo não fazer. Não existe banco de imagens livre e redistribuível
para 5 mil modelos; as fotos que se vê por aí são de sites com direito autoral.
Buscar na hora exigiria internet, espaço e assumir risco de licença. E o ganho é
pequeno: o aparelho está na mão do cliente, em cima do balcão. Se a intenção é
registro visual, o caminho é a foto de entrada que o app já tira, usada hoje em
3 OS de 219.

### Como ficou

1. **Catálogo embutido**: 5.301 modelos em 126 KB, gerado por
   `tools/make_models.py` a partir da lista pública do Google mais os iPhones
   escritos à mão. Não depende de internet. A busca aceita os termos em qualquer
   ordem e casa por início de palavra; leva 48 microssegundos por consulta.
2. **Sugestão no campo de modelo**: o histórico da loja vem primeiro, o catálogo
   completa. Digitar livre continua valendo, o que é obrigatório porque 27% das
   OS são console, TV e eletrodoméstico, que não estão no catálogo.
3. **Serviço reconhecido** a partir do defeito, por dez regras montadas em cima
   do texto real da loja (troca de tela, bateria, conector de carga, formatação e
   vírus, oxidação, áudio, câmera, botões, placa, não liga).
4. **Memória de preço** embaixo do valor cobrado: mostra o que foi cobrado e o
   custo das últimas vezes naquele modelo e serviço, e um toque preenche os dois
   campos.
5. As chaves de modelo e serviço são derivadas do que já está gravado, então as
   219 OS antigas entram na memória sozinhas, sem ninguém redigitar nada e sem
   depender do servidor.

## 12. WhatsApp da loja — FEITO em 0.11.0

A seção 6 dizia que depois da entrega não existe nada. Faltava o passo anterior:
o aparelho fica pronto e o cliente não sabe. Na base as 7 OS aprovadas estão
paradas há mais de sete dias — todas elas, sem exceção — cada uma um aparelho na
prateleira e um valor que não entrou, e há outras 92 esperando aprovação.
A 0.9.0 já montava a mensagem, mas o envio saía pelo `wa.me`: trocar de
janela, esperar o WhatsApp Web carregar e apertar enviar, uma vez por cliente.

### O que foi feito

1. **Status novo "Pronto para retirada"**, entre Aprovado e Entregue. Ele separa
   "está em conserto" de "está pronto e esperando o cliente", que era exatamente
   a informação que não existia em lugar nenhum.
2. **WhatsApp da loja pareado uma vez**, em Configurações > WhatsApp: um QR lido
   pelo celular da loja, como o WhatsApp Web. A sessão fica neste computador, ao
   lado do banco, e sobe sozinha nas próximas vezes.
3. **Marcar a OS como pronta oferece o aviso na hora**, com o texto à vista antes
   de sair: nome do cliente, número da OS, aparelho e valor. Três saídas —
   enviar pelo WhatsApp da loja, abrir o WhatsApp do computador, ou "Agora não".
4. **O envio fica registrado** na OS (data do contato) e num registro próprio,
   que alimenta o teto diário.

### Como foi feito, e por quê

O protocolo roda **num processo separado**, o mesmo executável chamado de novo
com `--whatsapp-bridge`, conversando por uma porta local protegida por um
segredo sorteado a cada execução ([whatsapp_bridge.go](whatsapp_bridge.go)). O
motivo é direto: uma falha na pilha de protocolo derruba o processo, e o processo
que não pode cair é o que registra as OS no balcão. Com a ponte separada, o pior
caso é ficar sem enviar mensagem, e não perder um atendimento pela metade.

Não há servidor, Docker nem serviço na nuvem para manter: é uma biblioteca Go
dentro do próprio app. O custo é tamanho: o executável fica em 52 MB, cerca de
7 MB a mais do que a 0.10.0.

### Os limites, ditos abertamente

É um **cliente não oficial** do WhatsApp — o mesmo caminho que o Syne Spark usa.
O que faz o WhatsApp bloquear um número é disparo para quem não pediu contato.
Por isso o app só envia para cliente com OS aberta, sobre a OS dele, com teto de
60 mensagens por dia e aviso quando é fora do horário comercial. Ainda assim: o
número pareado deve ser o **da loja, nunca o pessoal do dono**.

No celular a tela de pareamento não aparece: a sessão mora no computador do
balcão. O aparelho do dono continua avisando pelo caminho antigo, o `wa.me`.

## 13. Como eu sequenciaria

A ordem abaixo é por retorno dividido por esforço, e agrupada pelo que pode
sair junto. O critério é sempre o mesmo: quanto tempo de balcão economiza ou
quanto dinheiro destrava.

### Lote 1 — só no app, sem mexer no servidor — FEITO em 0.9.0

Ataca o funil parado e a digitação. É onde está o retorno imediato.

1. WhatsApp com mensagem pronta na linha da OS e dentro dela.
2. Idade da OS visível na lista, e o sino contando o que está atrasado.
3. Destravar o Tab, fazer os atalhos funcionarem dentro dos campos e arrumar a
   ordem do foco para o valor ficar alcançável.
4. "Salvar e nova OS", "salvar e imprimir", e lembrar a impressora.
5. Sugestão por histórico no defeito e no modelo; tipo de aparelho em lista.
6. Aviso ao sair com cadastro pela metade.
7. Trocar status direto na lista, sem abrir a OS.
8. Busca com espera curta e fora da thread da interface.
9. Campo próprio para a senha do aparelho, fora do recibo do cliente.
10. Prazo de garantia na OS e no recibo, com aviso quando o aparelho volta dentro
    do prazo.
11. Corrigir os modelos de checklist duplicados.

### Lote 2 — precisa de um deploy do worker — FEITO em 0.12.0

Saiu tudo num deploy só, como planejado:

- `delivered_at`, campos de pagamento e o custo pedido na hora da entrega
  (seção 3);
- trava de revisão na gravação (2.7), isolamento de erro por alteração nos dois
  lados (2.8), conferência de papel no push (1.5), troca de senha que vale de
  verdade (1.6) e limite de tentativas de login (1.8);
- de quebra, `device_key` e `contacted_at` passaram a sincronizar. Eles existiam
  desde a 0.9.0 mas não tinham coluna no servidor, então a senha do aparelho e a
  data da cobrança nunca saíam do computador onde foram digitadas.

O que **não** entrou: a compactação do diário de alterações (2.9). A primeira
carga de um aparelho novo continua crescendo com o histórico de edições, e não
com a quantidade de registros.

### Lote 3 — antes de publicar qualquer coisa para Android

Botão Voltar, retorno visual no toque, teclado numérico e as três armadilhas do
script de build. Sem isso o APK novo é pior que o 0.7.1 que está no ar.

### Lote 4 — decisões suas, não minhas

- ~~Separar "esperando orçamento" de "esperando o cliente".~~ Feito em 0.13.0,
  sem mexer na lista de status (ver seção 2).
- Checklist e fotos: insistir com câmera no celular, ou recolher da tela. A base
  de 24/09 diz que **3 OS de 220** têm checklist e **3** têm foto, com quatro
  modelos de checklist cadastrados. É o candidato mais forte a sair da tela.
- Contas por técnico: hoje existe uma conta só, e todo o controle de papel,
  responsável e "só minhas OS" está sem uso.

### Lote 5 — segurança, fora do código

Continua pendente e não depende de programação: revogar as chaves do R2 e fechar
a leitura pública do bucket. Está no [AUDITORIA.md](AUDITORIA.md), itens 1.1 e 1.2.

---

## 14. Revisão de 24/09/2026: o gargalo não é a bancada

Depois de 0.9.0 a 0.12.0, medi a base de novo. O quadro mudou de lugar.

**A oficina é rápida.** Das 96 OS entregues, a mediana entre abrir e entregar é
de **2,5 dias**, e 30% saem no mesmo dia. Capacidade não é o problema.

**O problema é a fila que não anda.**

| | |
|---|---|
| Faturado em 2,5 meses | R$ 15.860 |
| Orçado e parado na fila (99 OS) | **R$ 13.949** |
| Parado há mais de 30 dias | 30 OS |
| Com telefone utilizável | 94 de 99 |

A fila parada vale **88% de tudo que a loja faturou no período**. E ela é duas
filas: 46 esperando a loja dar um preço, 46 esperando o cliente responder. Essa
separação saiu na 0.13.0 (seção 2, item 3).

**O que falta é fechar o ciclo da segunda fila.** A cobrança por WhatsApp
existe desde a 0.9.0 e o envio pelo número da loja desde a 0.11.0 — as duas
publicadas em 24/09, ou seja, a loja teve horas de uso, não semanas. Não há
dado de uso ainda, e é por isso que o passo seguinte não é mais uma forma de
enviar, e sim **receber**: a `whatsmeow` entrega mensagens recebidas por
`AddEventHandler`, então a ponte que já existe pode ler a resposta do cliente,
casar pelo telefone com a OS aberta dele e mudar o status.

Regras acertadas com o dono para esse passo:

- Um "sim" aprova em **qualquer valor** — o cliente respondeu a uma mensagem que
  dizia o preço, e isso é autorização. A resposta literal e o horário ficam
  gravados na OS como prova.
- **Menos** quando o "sim" vem acompanhado de adiamento: *"sim, espera um
  pouco"*, *"sim, entendi, me dá um tempo pra pensar"*. Aprova só quando há
  afirmação **e** nenhuma marca de hesitação ou pergunta; na dúvida, um humano lê.
- O bot só responde a número que tem OS aberta na loja. Número desconhecido não
  recebe nada — responder a qualquer um é o padrão que faz o WhatsApp bloquear.
- Cadastro de cliente novo pelo WhatsApp fica para depois: abre a porta para
  qualquer número criar registro, e a entrada de clientes não é o gargalo.
- A ponte roda no PC do balcão, então o bot só responde com o app aberto. Isso é
  intencional: vira horário comercial de graça.

### O que os números dizem para não fazer

- **Retenção** (8% dos clientes voltam, 16 de 204) é a maior oportunidade depois
  disso, e a mais barata. Mas depende do WhatsApp estar em uso de verdade
  primeiro.
- **Dispersão de preço** continua: das 8 combinações (modelo, serviço) que se
  repetem, 5 variam mais de 30% — tela de A14 por R$ 250 e por R$ 350, tela de
  G60 por R$ 270 e por R$ 550. A memória de preço da 0.10.0 ataca isso, mas só
  quando o par se repete, e hoje repetem 8. Deixar acumular.
- **Papéis e técnico responsável**: 1 usuário cadastrado, 2 OS com técnico
  nomeado em 220. A maquinaria inteira está sem uso; não construir mais ali.

---

## 15. O campo de anotações, lido de ponta a ponta — FEITO em 0.14.0

Vasculhei as 94 anotações da base para descobrir o que a loja precisa e o app
não oferece. O campo de texto livre mais usado é onde as necessidades sem
resposta aparecem.

| O que a anotação guarda | OS |
|---|---|
| **A senha do aparelho** | **51** |
| Lembrete de "orçar" | 10 |
| Prazo ou dia combinado | 5 |
| Pagamento parcial | 1 |
| Peça encomendada, fornecedor | 0 |
| Garantia, retorno | 0 |

Três conclusões.

**A primeira virou correção urgente.** 54% das anotações guardam a senha do
aparelho, e a anotação era impressa no comprovante do cliente enquanto o
formulário prometia o contrário. Está no [AUDITORIA.md](AUDITORIA.md), item 1.10.

**A segunda validou o que saiu na 0.13.0.** Dez OS usam a palavra "ORÇAR"
escrita na anotação como lista de tarefas, porque o app não tinha onde dizer
isso. A fila "Preciso orçar" é essa lista.

**A terceira é um limite do modelo de pagamento que fiz na 0.12.0.** Uma OS
registra *"foi pago 100 dos 220 da tela e conector, falta 150"*. O modelo é
binário — pago ou devendo — e sinal/entrada existe na vida real. Um caso só, não
é urgente, mas o modelo vai precisar disso.

E uma ausência que informa: **nenhuma anotação fala de peça encomendada ou
fornecedor**. Se a demora nas 46 OS sem orçamento fosse espera de cotação de
peça, apareceria aqui. Não aparece.

### Padrão de desbloqueio

A base tenta descrever desenho com palavras: `SENHA: L`, `SENHA M`, `senha tudo
quadrado`. Nenhuma dessas devolve o desenho depois. Na 0.14.0 o operador desenha
o padrão nos nove pontos, dentro do app, e pode repetir o movimento quantas
vezes precisar para conferir.

A sequência mora no `device_key` que já existe, com um prefixo (`padrao:1-2-3-6-9`),
então sincroniza desde a 0.12.0 **sem coluna nova, sem migração e sem deploy** —
a mesma disciplina da separação de filas da 0.13.0.

Sobre gerar um GIF do movimento, que foi a ideia original: a repetição dentro do
app entrega o mesmo ("conferir a qualquer momento") sem transformar a
credencial do cliente num arquivo solto, que é justamente o problema que esta
versão fecha. Se ainda assim o GIF for útil para algum caso, é fácil somar.

---

## O que eu não recomendo fazer agora

- **Otimizar busca ou banco.** Está medido: há anos de folga.
- **Novos relatórios.** O relatório que existe já não fecha, porque falta custo,
  pagamento e data de entrega. Mais relatório em cima de dado furado piora.
- **Mais campos por precaução.** Dos que existem, IMEI é preenchido em 2 OS de
  219 e acessórios em 15. Campo novo só com evidência de uso, como a senha do
  aparelho, que 89 OS já escrevem no lugar errado.
