package service

import "testing"

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
