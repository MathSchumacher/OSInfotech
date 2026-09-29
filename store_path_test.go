package main

import (
	"errors"
	"testing"
)

func TestDatabaseBaseDirUsesAndroidPrivateFilesDir(t *testing.T) {
	base, err := databaseBaseDir("android", "/data/user/0/br.com.osinfotech.mobile/files", func() (string, error) {
		return "", errors.New("should not be used on Android")
	})
	if err != nil {
		t.Fatalf("databaseBaseDir() error = %v", err)
	}
	if base != "/data/user/0/br.com.osinfotech.mobile/files" {
		t.Fatalf("databaseBaseDir() = %q", base)
	}
}

func TestDatabaseBaseDirRejectsMissingAndroidFilesDir(t *testing.T) {
	_, err := databaseBaseDir("android", "", func() (string, error) {
		return "unused", nil
	})
	if err == nil {
		t.Fatal("databaseBaseDir() should reject a missing Android FILESDIR")
	}
}
