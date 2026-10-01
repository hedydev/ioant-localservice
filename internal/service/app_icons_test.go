package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAssetImagePlatformAndPixels(t *testing.T) {
	cases := []struct {
		image    assetCatalogImage
		platform string
	}{
		{assetCatalogImage{Idiom: "universal", Platform: "ios"}, "ios"},
		{assetCatalogImage{Idiom: "iphone"}, "ios"},
		{assetCatalogImage{Idiom: "ipad"}, "ios"},
		{assetCatalogImage{Idiom: "mac"}, "macos"},
		{assetCatalogImage{Platform: "macos"}, "macos"},
	}
	for _, tc := range cases {
		if got := assetImagePlatform(tc.image); got != tc.platform {
			t.Fatalf("assetImagePlatform(%+v) = %q, want %q", tc.image, got, tc.platform)
		}
	}
	if got := appIconPixels("512x512", "2x"); got != 1024*1024 {
		t.Fatalf("appIconPixels = %d, want %d", got, 1024*1024)
	}
}

func TestPNGBytes(t *testing.T) {
	valid := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	if !pngBytes(valid) {
		t.Fatal("expected valid PNG signature")
	}
	if pngBytes([]byte("not png")) {
		t.Fatal("unexpected PNG signature match")
	}
}


func TestRefreshProjectIconsReadsGeneratedCatalogPNG(t *testing.T) {
	root := t.TempDir()
	setDir := filepath.Join(root, "App", "Assets.xcassets", "AppIcon.appiconset")
	if err := os.MkdirAll(setDir, 0755); err != nil {
		t.Fatal(err)
	}
	contents := []byte(`{"images":[{"filename":"AppIcon-1024.png","idiom":"universal","platform":"ios","size":"1024x1024"}],"info":{"author":"xcode","version":1}}`)
	contentsPath := filepath.Join(setDir, "Contents.json")
	if err := os.WriteFile(contentsPath, contents, 0644); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	if err := os.WriteFile(filepath.Join(setDir, "AppIcon-1024.png"), png, 0644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"init", "-q"},
		{"add", filepath.ToSlash(filepath.Join("App", "Assets.xcassets", "AppIcon.appiconset", "Contents.json"))},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	a := &App{data: t.TempDir()}
	a.refreshProjectIcons("demo", root)
	got, err := os.ReadFile(a.projectIconPath("demo", "ios"))
	if err != nil {
		t.Fatalf("project icon was not cached: %v", err)
	}
	if !pngBytes(got) {
		t.Fatal("cached project icon is not PNG")
	}
}

func TestReadCatalogPNGRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	setDir := filepath.Join(root, "Assets.xcassets", "AppIcon.appiconset")
	if err := os.MkdirAll(setDir, 0755); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	if err := os.WriteFile(filepath.Join(root, "outside.png"), png, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readCatalogPNG(root, filepath.ToSlash(filepath.Join("Assets.xcassets", "AppIcon.appiconset")), "../outside.png"); err == nil {
		t.Fatal("expected traversal filename to be rejected")
	}
}
