package main

import (
	"path/filepath"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// pageKind identifica cada tela do app. Listas e editores sao paginas
// separadas: a lista de OS e uma pagina, a OS aberta e outra.
type pageKind int

const (
	pageHome pageKind = iota
	pageOrders
	pageOrderEditor
	pageCustomers
	pageCustomerEditor
	pageTerms
	pageTermEditor
	pageReports
	pageSettings
	pageSearch
)

// navSection e o item da navegacao principal que fica destacado.
type navSection int

const (
	navHome navSection = iota
	navOrders
	navCustomers
	navTerms
	navReports
	navSettings
)

// sectionOf diz qual item de navegacao corresponde a cada pagina.
func sectionOf(kind pageKind) navSection {
	switch kind {
	case pageOrders, pageOrderEditor:
		return navOrders
	case pageCustomers, pageCustomerEditor:
		return navCustomers
	case pageTerms, pageTermEditor:
		return navTerms
	case pageReports:
		return navReports
	case pageSettings:
		return navSettings
	default:
		return navHome
	}
}

type mainUI struct {
	app    fyne.App
	store  *Store
	window fyne.Window
	dbPath string

	currentUser AppUser
	loginNotice string // aviso mostrado na proxima tela de acesso

	// Campos da tela de acesso, guardados para posicionar o cursor.
	loginEntry    *dbsEntry
	passwordEntry *dbsEntry

	// ---- casca (shell) ----
	content      fyne.CanvasObject // raiz mostrada apos o login
	pageHost     *fyne.Container   // area onde a pagina ativa e montada
	pages        map[pageKind]fyne.CanvasObject
	current      pageKind
	navItems     map[navSection]*navItem
	mobileNav    map[navSection]*mobileNavItem
	toast        *toastHost
	userName     *themedText
	userRole     *themedText
	userAv       *avatar
	syncPill     *syncIndicator
	globalSearch *globalSearchBox
	ordersBadge  *navBadge
	alertBell    *alertBell
	userChip     fyne.CanvasObject
	chipAv       *avatar
	chipName     *themedText
	chipSub      *themedText

	// ---- dados carregados ----
	orders  []ServiceOrder
	clients []CustomerProfile
	terms   []ServiceTerm

	// suppressSelect evita que a selecao programatica de uma linha (ao voltar
	// para a lista) seja tratada como clique e reabra o editor.
	suppressSelect bool

	// syncRunning impede duas sincronizacoes ao mesmo tempo (a primeira carga
	// demora minutos e o app dispara sync ao salvar qualquer coisa).
	syncRunning atomic.Bool

	// ---- lista de OS ----
	currentID int64
	// orderFormSeq muda toda vez que o formulario troca de OS. Uploads em
	// andamento comparam com ele para nao anexar a foto na OS errada.
	orderFormSeq      uint64
	orderStatusFilter string // "" = todas
	searchEntry       *dbsEntry
	statusFilter      *widget.Select // controle usado no celular
	statusChips       *chipGroup     // controle usado no desktop
	onlyMineCheck     *widget.Check
	orderList         *widget.List
	listSummary       *themedText
	ordersEmpty       fyne.CanvasObject
	ordersTable       fyne.CanvasObject
	ordersTitleSub    *themedText

	// ---- editor de OS ----
	detailTitle    *themedText
	detailMeta     *themedText
	detailStatus   *statusPill
	statusSelect   *widget.Select
	customerPicker *customerPicker
	avatarImage    *canvas.Image
	avatarStatus   *widget.Label
	avatarBox      fyne.CanvasObject
	cpfEntry       *dbsEntry
	nameEntry      *dbsEntry
	phoneEntry     *dbsEntry
	deviceEntry    *dbsEntry
	modelEntry     *dbsEntry
	serialEntry    *dbsEntry
	deviceKeyEntry *dbsEntry
	// Texto do formulario na ultima gravacao, para saber se ha algo a perder.
	orderFormBaseline string
	orderSearchSeq    atomic.Uint64
	// Dica de preco: o que foi cobrado da ultima vez neste aparelho.
	priceHintBox *fyne.Container
	priceHintSeq atomic.Uint64
	// Ponte com o WhatsApp da loja (so no computador do balcao).
	whatsapp       *whatsappService
	issueEntry     *dbsEntry
	accessEntry    *dbsEntry
	notesEntry     *dbsEntry
	amountEntry    *dbsEntry
	costEntry      *dbsEntry
	costField      fyne.CanvasObject
	netAmountLabel *widget.Label
	netAmountTitle *themedText
	// Bloco de cobranca por WhatsApp dentro da OS.
	followUpBox    *fyne.Container
	followUpBtn    *dbsButton
	contactedLabel *themedText
	// Contadores da foto em carregamento: a resposta que chega atrasada nao pode
	// pintar a foto de um cliente no cadastro de outro.
	avatarPreviewSeq       uint64
	clientAvatarPreviewSeq uint64
	netAmountText          *themedText
	techLabel              *widget.Label
	deleteOSButton         *dbsButton
	orderSaveBtn           *dbsButton

	currentCostCents      int64
	currentTechnicianID   int64
	currentTechnicianName string

	// Fechamento da OS: o formulario nao tem caixa para estes tres, entao eles
	// viajam na sombra para o collectOrder nao apaga-los a cada gravacao.
	currentDeliveredAt   time.Time
	currentPaidAt        time.Time
	currentPaymentMethod string
	deliveryBox          *fyne.Container
	deliveryLabel        *widget.Label

	// Aviso de senha digitada no campo errado, e a tarja da lista com as OS
	// antigas que ainda precisam de olho humano.
	notePasswordBox   *fyne.Container
	ordersPasswordBox *fyne.Container

	// O valor real da senha do aparelho. Quando e um desenho, o campo de texto
	// mostra so o resumo, entao ele nao pode ser a fonte da verdade.
	currentDeviceKey string
	patternBtn       *dbsButton
	deviceKeyRow     *fyne.Container

	contactNameEntry  *dbsEntry
	contactPhoneEntry *dbsEntry
	paidEntry         *dbsEntry
	phoneWarning      *fyne.Container

	customerLookupSeq      atomic.Uint64
	suppressCustomerLookup bool
	lastAutoCPF            string
	lastAutoPhone          string
	currentAvatarURL       string
	currentAvatarKey       string

	checklistTemplates  []ChecklistTemplate
	checklistSelect     *widget.Select
	checklistItemsBox   *fyne.Container
	checklistItemStates []*widget.Select
	currentChecklist    OrderChecklist
	photosBox           *fyne.Container
	currentPhotos       []OrderPhoto

	// ---- clientes ----
	clientSearchEntry   *dbsEntry
	clientList          *widget.List
	clientSummary       *themedText
	clientsEmpty        fyne.CanvasObject
	clientsDuplicateBox *fyne.Container
	clientsTable        fyne.CanvasObject
	clientsTitleSub     *themedText
	clientDetailTitle   *themedText
	clientDetailMeta    *themedText
	clientNameEntry     *dbsEntry
	clientCPFEntry      *dbsEntry
	clientPhoneEntry    *dbsEntry
	clientAvatarImage   *canvas.Image
	clientAvatarStatus  *widget.Label
	clientAvatarBox     fyne.CanvasObject
	clientHistoryBox    *fyne.Container
	clientSummaryBox    *fyne.Container
	currentClientKey    string
	clientAvatarURL     string
	clientAvatarKey     string
	deleteClientBtn     *dbsButton

	// ---- termos ----
	currentTermID           int64
	termSearchEntry         *dbsEntry
	termDocumentFilter      *widget.Select
	termChips               *chipGroup
	termDocumentFilterValue string
	termList                *widget.List
	termSummary             *themedText
	termsEmpty              fyne.CanvasObject
	termsTable              fyne.CanvasObject
	termsTitleSub           *themedText
	termDetailTitle         *themedText
	termDetailMeta          *themedText
	termDetailStatus        *statusPill
	termTypeSelect          *widget.Select
	termStatusSelect        *widget.Select
	termSellerNameEntry     *dbsEntry
	termSellerPicker        *customerPicker
	termSellerCPFEntry      *dbsEntry
	termSellerRGEntry       *dbsEntry
	termSellerPhoneEntry    *dbsEntry
	termSellerAddressEntry  *dbsEntry
	termBuyerCompanyEntry   *dbsEntry
	termBuyerDocumentEntry  *dbsEntry
	termBuyerAddressEntry   *dbsEntry
	termBuyerPhoneEntry     *dbsEntry
	termDeviceKindEntry     *dbsEntry
	termBrandEntry          *dbsEntry
	termModelEntry          *dbsEntry
	termColorEntry          *dbsEntry
	termIMEI1Entry          *dbsEntry
	termIMEI2Entry          *dbsEntry
	termAccessoriesEntry    *dbsEntry
	termDeclarationEntry    *dbsEntry
	termAmountEntry         *dbsEntry
	termPaymentMethodEntry  *dbsEntry
	termCityEntry           *dbsEntry
	termDateEntry           *dbsEntry
	termPDFStatus           *widget.Label
	termSignedStatus        *widget.Label
	termPreview             *termPreview
	termLookupSeq           atomic.Uint64
	suppressTermLookup      bool
	lastTermAutoCPF         string
	lastTermAutoPhone       string
	deleteTermBtn           *dbsButton

	// ---- inicio ----
	homeTiles          map[string]*statTile
	homeAttention      *fyne.Container
	homeRecent         *fyne.Container
	homeAttentionTitle *themedText
	homeRecentTitle    *themedText
	homeOrdersBox      *fyne.Container
	homeOrdersEmpty    fyne.CanvasObject
	homeStatus         *homeStatusCard
	homeGreeting       *themedText
	homeDateText       *themedText

	// ---- relatorios / configuracoes ----
	reportPage   *reportPage
	settingsPage *settingsPage
}

func newMainUI(app fyne.App, store *Store, win fyne.Window, dbPath string) *mainUI {
	ui := &mainUI{
		app:       app,
		store:     store,
		window:    win,
		dbPath:    dbPath,
		pages:     map[pageKind]fyne.CanvasObject{},
		navItems:  map[navSection]*navItem{},
		mobileNav: map[navSection]*mobileNavItem{},
		homeTiles: map[string]*statTile{},
	}
	ui.whatsapp = newWhatsAppService(filepath.Dir(dbPath))
	ui.toast = newToastHost()
	ui.buildOrderWidgets()
	ui.buildCustomerWidgets()
	ui.buildTermForm()
	ui.content = ui.buildShell()
	ui.clearForm()
	ui.clearCustomerForm()
	return ui
}

// pageHostSet troca o conteudo da area de pagina.
func (ui *mainUI) pageHostSet(obj fyne.CanvasObject) {
	if ui.pageHost == nil {
		ui.pageHost = container.NewStack()
	}
	ui.pageHost.Objects = []fyne.CanvasObject{obj}
	ui.pageHost.Refresh()
}

// selectSilently seleciona uma linha sem disparar a abertura do editor.
func (ui *mainUI) selectSilently(list *widget.List, index int) {
	if list == nil {
		return
	}
	ui.suppressSelect = true
	defer func() { ui.suppressSelect = false }()
	if index < 0 {
		list.UnselectAll()
		return
	}
	list.Select(index)
}

// notify mostra um toast curto (sucesso por padrao).
func (ui *mainUI) notify(message string) {
	ui.notifyTone(message, toneSuccess)
}

func (ui *mainUI) notifyTone(message string, tone statusTone) {
	if ui.toast == nil {
		return
	}
	ui.toast.ShowMessage(message, tone)
}
