package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "osinfotech.db"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestLocalUserAuthenticationAndRoles(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	admin, err := store.BootstrapAdministrator(ctx, "Patrao", "admin", "senha-segura")
	if err != nil {
		t.Fatalf("BootstrapAdministrator() error = %v", err)
	}
	if !admin.IsAdmin() || !admin.Active {
		t.Fatalf("bootstrap admin = %#v", admin)
	}

	if _, err := store.Authenticate(ctx, "admin", "senha-segura"); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if _, err := store.Authenticate(ctx, "admin", "senha-incorreta"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidCredentials", err)
	}

	technician := AppUser{Name: "Tecnico", Login: "tecnico", Role: roleTechnician, Active: true}
	if err := store.SaveUser(ctx, &technician, "senha-tecnico"); err != nil {
		t.Fatalf("SaveUser() error = %v", err)
	}
	if technician.ID == 0 {
		t.Fatal("SaveUser() did not set technician ID")
	}
	if _, err := store.Authenticate(ctx, "tecnico", "senha-tecnico"); err != nil {
		t.Fatalf("Authenticate() technician error = %v", err)
	}

	admin.Active = false
	if err := store.SaveUser(ctx, &admin, ""); err == nil {
		t.Fatal("SaveUser() allowed disabling the only active admin")
	}
}

func TestOnlyAdministratorCanCreateAndPromoteUsers(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	admin, err := store.BootstrapAdministrator(ctx, "Patrao", "admin", "senha-segura")
	if err != nil {
		t.Fatalf("BootstrapAdministrator() error = %v", err)
	}

	blocked := AppUser{Name: "Nao permitido", Login: "bloqueado", Role: roleAdministrator, Active: true}
	if err := store.SaveUserAsAdministrator(ctx, admin, &blocked, "senha-segura"); err == nil {
		t.Fatal("SaveUserAsAdministrator() allowed creating an administrator directly")
	}

	technician := AppUser{Name: "Tecnico", Login: "tecnico", Role: roleTechnician, Active: true}
	if err := store.SaveUserAsAdministrator(ctx, admin, &technician, "senha-tecnico"); err != nil {
		t.Fatalf("SaveUserAsAdministrator() create technician error = %v", err)
	}

	technician.Role = roleAdministrator
	if err := store.SaveUserAsAdministrator(ctx, admin, &technician, ""); err != nil {
		t.Fatalf("SaveUserAsAdministrator() promote technician error = %v", err)
	}

	events, err := store.ListAuditEvents(ctx, 10)
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 2 || events[0].Action != "usuario.promovido_administrador" || events[1].Action != "usuario.criado" {
		t.Fatalf("audit events = %#v", events)
	}
}

func TestFinancialTotalsOnlyIncludeDeliveredOrders(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	orders := []ServiceOrder{
		{Status: statusPending, Customer: "A", Phone: "1", Device: "Celular", Issue: "Tela", AmountCents: 10000, CostCents: 2000},
		{Status: statusApproved, Customer: "B", Phone: "2", Device: "TV", Issue: "Imagem", AmountCents: 20000, CostCents: 7000},
		{Status: statusDelivered, Customer: "C", Phone: "3", Device: "PC", Issue: "Fonte", AmountCents: 30000, CostCents: 11000},
		{Status: statusDelivered, Customer: "D", Phone: "4", Device: "Notebook", Issue: "Bateria", AmountCents: 45000, CostCents: 12000},
	}
	for i := range orders {
		if err := store.Save(ctx, &orders[i]); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	totals, err := store.FinancialTotals(ctx)
	if err != nil {
		t.Fatalf("FinancialTotals() error = %v", err)
	}
	if totals.Orders != 2 || totals.GrossCents != 75000 || totals.CostCents != 23000 || totals.NetCents != 52000 {
		t.Fatalf("FinancialTotals() = %#v", totals)
	}
}
