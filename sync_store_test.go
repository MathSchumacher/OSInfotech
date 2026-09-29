package main

import (
	"context"
	"testing"
)

func TestSavingOrderQueuesCustomerAndOrderForD1(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	admin, err := store.BootstrapAdministrator(ctx, "Admin", "admin", "senha-segura")
	if err != nil {
		t.Fatalf("BootstrapAdministrator() error = %v", err)
	}

	order := ServiceOrder{
		Status:       statusPending,
		Customer:     "Cliente D1",
		CPF:          "12345678900",
		Phone:        "51999990000",
		Device:       "Celular",
		Issue:        "Troca de tela",
		AmountCents:  15000,
		TechnicianID: admin.ID,
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	changes, err := store.PendingSyncChanges(ctx, 10)
	if err != nil {
		t.Fatalf("PendingSyncChanges() error = %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("queued changes = %d, want 2", len(changes))
	}
	seen := map[string]bool{}
	for _, change := range changes {
		seen[change.EntityType] = true
		if change.Action != "upsert" || change.BaseRevision != 0 || change.EntityRemote == "" {
			t.Fatalf("invalid queued change: %#v", change)
		}
	}
	if !seen[syncEntityCustomer] || !seen[syncEntityOrder] {
		t.Fatalf("expected customer and order changes, got %#v", changes)
	}
}

func TestApplyRemoteCustomerUpdatesLocalCache(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	remote := remoteCustomer{
		ID:        "3f0c9a11-6f11-42ce-95da-7aaeb566bf10",
		Name:      "Cliente remoto",
		Document:  "98765432100",
		Phone:     "51988880000",
		Revision:  4,
		CreatedAt: "2026-06-23T12:00:00Z",
		UpdatedAt: "2026-06-23T13:00:00Z",
	}
	if err := store.applyRemoteCustomer(ctx, remote); err != nil {
		t.Fatalf("applyRemoteCustomer() error = %v", err)
	}
	profiles, err := store.ListCustomerProfiles(ctx, "")
	if err != nil {
		t.Fatalf("ListCustomerProfiles() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != remote.Name || profiles[0].CPF != remote.Document {
		t.Fatalf("profiles = %#v", profiles)
	}
	mapping, found, err := store.syncMappingByRemote(ctx, syncEntityCustomer, remote.ID)
	if err != nil || !found || mapping.Revision != remote.Revision {
		t.Fatalf("remote mapping = %#v, found=%v, err=%v", mapping, found, err)
	}
}
