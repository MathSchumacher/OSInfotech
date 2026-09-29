# DBS Assistência Técnica — Design System e Arquitetura de Telas

Remake da interface (setembro/2026). O app continua em Go + Fyne (desktop Windows
e Android), mas a organização das telas, a identidade visual e os componentes
foram refeitos do zero. Este documento é a referência para manter a consistência
nas próximas iterações.

## 1. Identidade

- **Nome exibido:** DBS Assistência Técnica (`appDisplayName`). O identificador
  técnico (`br.com.osinfotech.*`, pasta de dados, módulo Go) não muda: trocar
  quebraria a atualização por cima da instalação atual e o pacote Android.
- **Marca:** vermelho DBS puro sobre preto (tema escuro, padrão) e sobre
  branco (tema claro). Recortes em `assets/brand/` gerados de `assets/logo.png`
  e `assets/lightlogo.png`: `logo-*-wide` (barra lateral), `logo-*-hero` (login),
  `mark-*` (barra do celular).
- **Regra do vermelho:** vermelho significa marca e ação principal (item de
  navegação ativo, botão primário, badge de contagem). Nunca representa status
  de OS — status usam âmbar, azul, verde, coral e cinza.

## 2. Tokens (`ds_theme.go`)

| Token | Escuro | Claro | Uso |
|---|---|---|---|
| Canvas | `#121316` | `#F3F4F6` | fundo da janela |
| Surface | `#1A1C20` | `#FFFFFF` | cartões, barra lateral, topo |
| SurfaceRaised | `#23262B` | `#F6F7F9` | campos, botões secundários |
| SurfaceSunken | `#0E0F11` | `#EBEDF0` | cabeçalho de tabela, bloco de lucro |
| Border / BorderStrong | `#2B2F35` / `#3D424A` | `#E2E5E9` / `#C9CED5` | contornos |
| Ink / InkSecondary / InkMuted | `#EEF0F3` / `#A3ABB5` / `#6F7881` | `#14171B` / `#5B6570` / `#8B949E` | texto |
| Brand / Hover / Pressed | `#E5141C` / `#F42A32` / `#B80F16` | `#D31219` / `#E4191F` / `#A80C12` | ação principal |
| Success / Warning / Danger / Info / Neutral | `#3DCB80` `#F5B31B` `#FF6B57` `#5AA9FF` `#9AA3AD` | `#1D9A57` `#B96F00` `#D93A2B` `#1F6FEB` `#6B7480` | status (cada um tem versão `Soft` a 12–20%) |

O tema Fyne (`dbsTheme`) é derivado desses tokens. Componentes do design system
leem `tk()` em `Refresh()`, então trocar o tema (Configurações → Aparência ou o
ícone da paleta) repinta tudo sem reconstruir telas. A preferência fica em
`appearance_mode` (padrão `dark`).

**Tipografia:** IBM Plex Sans (Regular/Medium/SemiBold/Italic em
`assets/fonts`, licença OFL). Escala: display 28 · título 22 · seção 15 ·
corpo 13 · legenda 11 · micro 10 (rótulos em caixa alta).

**Raios:** 6 (pequeno), 10 (padrão), 14 (cartões), pílula (altura/2).

## 2b. Pele v2: atmosfera da marca

A segunda iteração (referências: `assets/example.png` e
`assets/examplelightmode.png`) substituiu as superfícies chapadas por uma
composição com atmosfera:

- **Fundo com arte** (`assets/brand/bg-dark.png` / `bg-light.png`, gerados por
  `tools/make_art.py`): preto com feixes de luz vermelhos em diagonal, trilhas
  de circuito e grades de pontos nos cantos; a versão clara usa os mesmos
  elementos em rosa suave. `newBackgroundArt()` fica atrás de toda casca e da
  tela de login.
- **Painéis de vidro** (`ds_raster.go`, `glassPanel`): rasterizados em Go por
  SDF de retângulo arredondado — preenchimento translúcido, borda de 1 pt,
  brilho externo colorido, gradiente horizontal e filete no topo. O Fyne não
  tem sombra, brilho nem gradiente com cantos arredondados; por isso o
  desenho é feito pixel a pixel e cacheado por tamanho/tema.
- **Tokens de vidro** (`Glass`, `GlassSunken`, `GlassBorder`, `GlassGlow`,
  `GlassStrong`) em `designTokens`. Campos e botões secundários usam
  fundos translúcidos no tema (`InputBackground`, `Button`).
- **Tiles de indicador**: no escuro, borda e brilho na cor do tom; no claro,
  filete colorido no topo e sombra suave. Botão redondo de seta no canto.
- **Item ativo da navegação**: pílula com gradiente vermelho e brilho
  (`brandPillGlass`).
- **Início** como painel de controle: banner com eyebrow, saudação com "!"
  vermelho e a arte da logo; quatro tiles; dois cartões com chip de ícone;
  tabela de OS recentes com botão de seta; painel escuro "Sua assistência
  técnica, conectada." com status da nuvem e "Sincronizar agora".
- **Barra lateral**: lockup da logo com bordas suaves, assinatura
  "Tecnologia que mantém você conectado" com filete vermelho, cartão do
  usuário com ponto verde e menu (Configurações, tema, sincronizar, trocar
  usuário). **Barra superior**: busca, pílula da nuvem, sino com contador de
  OS aguardando aprovação, chip do usuário.
- Cada página tem eyebrow vermelho e filete à esquerda do título
  (Operação, Cadastro, Documentos, Gestão, Sistema).

Regenerar a arte: `python tools/make_art.py` (precisa de Pillow e numpy).

## 3. Componentes (`ds_components.go`)

| Componente | Função |
|---|---|
| `themedRect`, `newFill`, `hairline` | superfícies e linhas que seguem o tema |
| `themedText` / `newEyebrow` | texto com tamanho livre; `SetTruncate(true)` corta com reticências (colunas) |
| `card`, `cardPadded`, `sunkenCard`, `tintedCard` | superfícies com borda/raio 14 |
| `primaryButton`, `secondaryButton`, `ghostButton`, `dangerButton`, `iconButton` | hierarquia de botões (ver 3b) |
| `tapCard` | superfície clicável com hover (linhas de lista, sugestões, opções) |
| `statusPill` + `orderStatusTone` / `termStatusTone` | status com ponto + tinta suave |
| `statTile` | indicador (chip de ícone, rótulo, valor grande, subtexto, barra colorida no topo) |
| `sectionHeader`, `field`, `fieldGrid`, `keyValue` | estrutura de formulários |
| `columnsLayout`, `tableHeader`, `cellText`, `vcenter` | tabelas por pesos de coluna |
| `avatar` | iniciais coloridas ou foto (cache em `images.go`) |
| `emptyState`, `kbdHint`, `toastHost` | estados vazios, atalho, feedback curto |

Na casca (`shell.go`): `navItem` + `navBadge` (barra lateral), `mobileNavItem`
(navegação inferior), `chipGroup`/`chip` (filtros), `syncIndicator` (nuvem).

## 3b. Botões e menus (`ds_button.go`, `ds_menu.go`)

O botão do Fyne é um retângulo de uma cor só, e era ele que dava ao app cara de
template. O botão da DBS é desenhado no mesmo rasterizador dos painéis
(`renderGlass`), com quatro estados, cada um em sua própria imagem:

| Estado | O que muda |
|---|---|
| repouso | gradiente na diagonal, luz na aresta de cima, sombra interna na base, feixe diagonal fraco |
| hover | gradiente mais claro, feixe desliza para a direita e ganha força, sombra projetada cresce |
| pressionado | vira baixo-relevo: a luz do topo vira sombra, a base perde a sombra, o conteúdo desce 1 pt |
| desabilitado | peça neutra, sem brilho e sem sombra |

A troca entre repouso e hover é animada em 130 ms com curva de saída: a camada
de hover é uma imagem separada cuja opacidade é interpolada. Nada de pisca-pisca
de troca de cor.

Recursos que o rasterizador ganhou para isso (todos opcionais, zero desliga):
`Diagonal`, `Sheen`/`SheenH` (luz interna a partir do topo), `InnerShade`/
`InnerShadeH` (sombra interna a partir da base), `Beam`/`BeamPos`/`BeamW` (feixe
diagonal no mesmo ângulo dos fachos do fundo), `Grain` (granulado fino, calculado
em pixels do dispositivo), `GlowOffsetY` (desloca o brilho para baixo e o
transforma em sombra projetada), `Chamfer` (corte de 45° no canto inferior
direito) e `Pad` (margem fixa da imagem, obrigatória quando o brilho muda de
tamanho entre estados — sem ela uma camada é esticada na caixa da outra).

**Suavizacao (vale para todo painel, botao e tile).** A cobertura da peca e
medida em pixels do dispositivo, nao em pontos logicos: a transicao tem sempre
1 pixel, em qualquer escala de tela. Medindo em pontos, como era antes, a mesma
borda saia mole numa tela a 150% e dura numa a 100%. A faixa da borda e suavizada
nas duas arestas; quando so a de fora era suavizada, a aresta de dentro virava um
degrau e serrilhava os cantos arredondados. E as imagens rasterizadas usam
reamostragem suave: o retangulo na tela quase nunca bate exatamente com o tamanho
em pixels da imagem, e o vizinho mais proximo comia pedacos da linha de 1 pixel,
deixando a borda falhada.

Regras de uso:

- **Primário** é a única peça sólida da tela, e a única com chanfro: o corte no
  canto é a assinatura da marca, não decoração repetida em tudo.
- **Destrutivo** nunca é sólido em repouso. O corpo fica neutro e quem avisa é o
  contorno, o texto e o brilho, que acendem no hover. Tingir o vidro com o tom de
  alerta deixa a peça marrom no escuro e cinza no claro.
- **Sombra é vermelho escuro**, não vermelho aceso: halo neon é o que faz um
  botão parecer barato.
- No tema claro, escurecer o corpo inteiro para indicar toque suja a cor. O
  pressionado é sempre baixo-relevo, nunca um fundo cinza.
- Todo botão tem cursor de mão, foco por teclado com anel próprio e aciona por
  Espaço e Enter.

O menu de ações (`showActionMenu`) segue o mesmo vocabulário: folha de vidro
quase opaca com sombra real, cabeçalho com quem está logado, linhas com ícone,
texto de apoio opcional e uma barra vermelha na lateral ao passar o ponteiro. A
ação destrutiva fica separada por um fio e em tom de alerta. A folha entra
descendo 6 pt em 120 ms.

Para revisar sem abrir o app: `DBS_SHOTS_DIR=<pasta> go test -run TestShotButtons`
e `-run TestShotUserMenu` gravam as folhas de estados nos dois temas.

## 4. Arquitetura de telas

```
Desktop                                   Celular
┌────────┬──────────────────────────┐     ┌──────────────────────┐
│ logo   │ busca global  [nuvem][+] │     │ marca  [nuvem][🔍][🎨]│
│ Início │──────────────────────────│     │ página               │
│ OS  ●2 │ página (título + corpo)  │     │                      │
│Clientes│                          │     │                      │
│ Termos │                          │     ├──────────────────────┤
│ GESTÃO │                          │     │ Início OS Clientes   │
│ Relat. │                          │     │ Termos Mais          │
│ Config.│                          │     └──────────────────────┘
│ usuário│                          │
└────────┴──────────────────────────┘
```

Cada funcionalidade tem sua seção e páginas próprias (`pageKind`):

- **Início** (`page_home.go`): saudação, 4 indicadores (clicáveis), OS
  aguardando aprovação há mais tempo, atividade recente.
- **Ordens de serviço** (`page_orders.go`): lista com busca + chips de status +
  "Só minhas OS" → editor em página própria (cliente, aparelho, checklist e
  fotos, observações; trilho com andamento e valores).
- **Clientes** (`page_customers.go`): lista com avatar → página do cliente com
  dados, histórico de OS e resumo (total, em aberto, entregues, total pago).
- **Termos** (`page_terms.go`): lista + filtros de documento → editor com
  documento, vendedor, comprador, equipamento, valores; trilho com PDF/assinado
  e prévia.
- **Relatórios** (`page_reports.go`, administrador): período, indicadores,
  OS entregues, exportação PDF/Word/Excel.
- **Configurações** (`page_settings.go`): Aparência, Equipe e auditoria, Nuvem
  e sincronização, Impressão 80 mm, Checklists, Padrões de termo,
  Atualizações, Sobre.

`showPage(kind)` monta a página na primeira vez e a mantém em cache; listas
recarregam ao entrar. `openOrder/openCustomer/openTerm` sempre chamam
`showPage` antes de carregar os dados (o editor precisa existir).

## 5. Facilitadores

- **Busca global** (`search_global.go`, Ctrl+K): clientes, OS e termos com o
  mesmo texto (nome, CPF, telefone, nº, IMEI, aparelho). Popup no desktop,
  página no celular.
- **Busca rápida de cliente no formulário** (`customer_picker.go`): sugestões
  abaixo do campo; escolher preenche nome/CPF/telefone/foto. Digitar um CPF ou
  telefone direto no campo reconhece o cliente e completa o resto.
- **Máscaras** (`inputs.go`): CPF/CNPJ, telefone e dinheiro no estilo maquininha
  (`1500` → `R$ 15,00`).
- **Atalhos** (desktop): Ctrl+K busca, Ctrl+N nova OS, Ctrl+S salva a página
  atual. Enter no login envia.
- **Acesso lembrado** (`session.go`): na tela de entrada há "Lembrar meu login
  neste aparelho" (preenche o login e põe o cursor na senha) e "Manter
  conectado" (o app abre já logado, sem digitar nada). As duas opções ficam em
  `fyne.Preferences`, ou seja, valem só naquele aparelho e nunca vão para a
  nuvem; **a senha nunca é guardada**. A sessão dura 30 dias e cai antes disso
  se o token da nuvem vencer, se o usuário for desativado ou se alguém usar
  "Trocar usuário" (que preserva o login lembrado). Configurações → "Acesso
  deste aparelho" permite rever, desligar ou "Esquecer este aparelho".
- **Seletor de imagens com miniaturas** (`image_picker.go`): no computador, o
  diálogo do Fyne mostra só um ícone com a extensão, então há um seletor
  próprio com grade de miniaturas, atalhos de pastas (Imagens, Downloads, Área
  de trabalho, Documentos, unidades), campo de caminho, duplo clique para
  confirmar e memória da última pasta. As fotos da OS aceitam **várias de uma
  vez**. As miniaturas são geradas em segundo plano (no máximo três por vez) e
  ficam em cache por caminho + data de modificação. No celular o Fyne delega ao
  seletor do próprio sistema, que já mostra as fotos.
- **Identidade do cliente** (`customers.go`, `customers_ui.go`): o cadastro é
  identificado pelo nome, então editar o nome **renomeia** o cliente e leva
  junto as OS e o vínculo com a nuvem, em vez de criar um segundo cadastro.
  Um CPF ou telefone só pode pertencer a um cliente: ao salvar, o app mostra
  quem já usa aquele número e oferece abrir ou unificar. Números de
  preenchimento (0000000000) são ignorados nessa checagem. Salvar uma OS com o
  CPF de outro cliente pergunta qual usar. A lista de Clientes avisa quando há
  cadastros repetidos e a revisão deixa escolher qual fica — o app nunca
  unifica sozinho.
- **Toasts** em vez de diálogos para salvar, excluir e sincronizar.
- **WhatsApp** a partir do telefone do cliente; **Nova OS para o cliente** a
  partir da página do cliente.

## 6. Como revisar visualmente

`ui_shots_test.go` renderiza todas as páginas, nos dois temas e nos dois
layouts, sem abrir janela:

```powershell
$env:DBS_SHOTS_DIR = "C:\temp\dbs-shots"
go test -run TestShot -count=1 .
```

Os PNGs ficam em `DBS_SHOTS_DIR` (`desktop-dark-02-orders.png`,
`mobile-light-06-customer.png`, ...). Use isso antes de qualquer mudança
visual grande.

## 7. Regras para novas telas

1. Toda tela nova é uma `pageKind` com `build...Page()` e entra em `buildPage`.
2. Cores só via tokens (`tk()`), nunca hex direto em telas.
3. Título com `ui.pageTitle(...)`; corpo em `card`s com `sectionHeader`.
4. Listas grandes em `widget.List` com linha densa (`cellText`, `vcenter`).
5. Feedback de sucesso em toast; erro de validação em toast `toneDanger`;
   erros de sistema em `dialog.ShowError`.
6. Ações destrutivas sempre confirmam e ficam à esquerda do botão primário.
7. Celular: `fieldGrid` vira coluna única; ações do cabeçalho quebram linha;
   botão principal fixo no rodapé (`mobileActionBar`).
