package main

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"0.1.1", "0.1.0", 1},
		{"v1.2.0", "1.1.9", 1},
		{"1.2", "1.2.0", 0},
		{"1.2.0", "1.2.1", -1},
		{"dev", "1.0.0", 0},
	}

	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if got != tt.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestResolveUpdateURL(t *testing.T) {
	got, err := resolveUpdateURL("https://cdn.example.com/os/latest.json", "OSInfotech-Setup-1.0.0.exe")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://cdn.example.com/os/OSInfotech-Setup-1.0.0.exe"
	if got != want {
		t.Fatalf("resolveUpdateURL = %q, want %q", got, want)
	}
}
