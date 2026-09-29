package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"
)

func writeReceiptHTML(order ServiceOrder, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	fileName := fmt.Sprintf("OS-%06d.html", order.ID)
	path := filepath.Join(dir, fileName)

	var buf bytes.Buffer
	if err := receiptTemplate.Execute(&buf, receiptView{
		IconDataURI: appIconDataURI(),
		Number:      fmt.Sprintf("%06d", order.ID),
		Status:      order.Status,
		CPF:         order.CPF,
		Customer:    order.Customer,
		Phone:       order.Phone,
		Device:      order.Device,
		BrandModel:  order.BrandModel,
		Serial:      order.Serial,
		Issue:       order.Issue,
		Accessories: order.Accessories,
		Notes:       order.Notes,
		Amount:      formatBRL(order.AmountCents),
		Technician:  order.TechnicianName,
		CreatedAt:   formatDate(order.CreatedAt),
		UpdatedAt:   formatDate(order.UpdatedAt),
	}); err != nil {
		return "", err
	}

	return path, os.WriteFile(path, buf.Bytes(), 0o644)
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("02/01/2006 15:04")
}

type receiptView struct {
	IconDataURI template.URL
	Number      string
	Status      string
	CPF         string
	Customer    string
	Phone       string
	Device      string
	BrandModel  string
	Serial      string
	Issue       string
	Accessories string
	Notes       string
	Amount      string
	Technician  string
	CreatedAt   string
	UpdatedAt   string
}

var receiptTemplate = template.Must(template.New("receipt").Parse(`<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="icon" type="image/png" href="{{.IconDataURI}}">
  <title>OS {{.Number}}</title>
  <style>
    :root { color-scheme: light; --ink: #18212b; --muted: #5e6a75; --line: #ccd5dd; --accent: #0f766e; }
    * { box-sizing: border-box; }
    body { margin: 0; padding: 32px; font-family: "Segoe UI", Calibri, sans-serif; color: var(--ink); background: #f3f6f8; }
    main { max-width: 880px; margin: 0 auto; background: #fff; border: 1px solid var(--line); box-shadow: 0 20px 60px rgba(24,33,43,.12); }
    header { display: flex; justify-content: space-between; gap: 24px; padding: 28px 32px; border-bottom: 4px solid var(--accent); }
    h1 { margin: 0; font-size: 28px; letter-spacing: 0; }
    h2 { margin: 26px 0 12px; font-size: 15px; text-transform: uppercase; letter-spacing: .08em; color: var(--accent); }
    .meta { text-align: right; color: var(--muted); line-height: 1.55; }
    .content { padding: 8px 32px 32px; }
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 28px; }
    .field { border-bottom: 1px solid var(--line); padding: 8px 0; min-height: 40px; }
    .label { display: block; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .06em; }
    .value { display: block; margin-top: 3px; font-size: 15px; white-space: pre-wrap; }
    .wide { grid-column: 1 / -1; }
    .amount { font-size: 22px; font-weight: 700; color: var(--accent); }
    footer { display: grid; grid-template-columns: 1fr 1fr; gap: 32px; padding-top: 42px; color: var(--muted); }
    .sign { border-top: 1px solid var(--line); padding-top: 8px; text-align: center; }
    @media print { body { background: #fff; padding: 0; } main { border: 0; box-shadow: none; } }
  </style>
</head>
<body>
  <main>
    <header>
      <div>
        <h1>Ordem de Servico</h1>
        <div>Loja tecnica de conserto de eletronicos</div>
      </div>
      <div class="meta">
        <strong>OS {{.Number}}</strong><br>
        Status: {{.Status}}<br>
        Criada: {{.CreatedAt}}<br>
        Atualizada: {{.UpdatedAt}}
      </div>
    </header>
    <section class="content">
      <h2>Cliente</h2>
      <div class="grid">
        <div class="field"><span class="label">Nome</span><span class="value">{{.Customer}}</span></div>
        <div class="field"><span class="label">CPF</span><span class="value">{{.CPF}}</span></div>
        <div class="field"><span class="label">Telefone</span><span class="value">{{.Phone}}</span></div>
        <div class="field"><span class="label">Valor</span><span class="value amount">{{.Amount}}</span></div>
      </div>

      <h2>Aparelho</h2>
      <div class="grid">
        <div class="field"><span class="label">Dispositivo</span><span class="value">{{.Device}}</span></div>
        <div class="field"><span class="label">Marca / modelo</span><span class="value">{{.BrandModel}}</span></div>
        <div class="field wide"><span class="label">Serie / IMEI / patrimonio</span><span class="value">{{.Serial}}</span></div>
      </div>

      <h2>Atendimento</h2>
      <div class="grid">
        <div class="field wide"><span class="label">Motivo / defeito relatado</span><span class="value">{{.Issue}}</span></div>
        <div class="field wide"><span class="label">Acessorios recebidos</span><span class="value">{{.Accessories}}</span></div>
		<div class="field wide"><span class="label">Responsavel tecnico</span><span class="value">{{.Technician}}</span></div>
      </div>

      <footer>
        <div class="sign">Assinatura do cliente</div>
        <div class="sign">Responsavel tecnico</div>
      </footer>
    </section>
  </main>
</body>
</html>`))
