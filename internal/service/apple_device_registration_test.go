package service

import (
	"testing"
	"time"
)

func TestAppleRegistrationStatus(t *testing.T) {
	cases := map[string]string{
		"ENABLED":    "registered",
		"PROCESSING": "processing",
		"DISABLED":   "disabled",
		"INELIGIBLE": "ineligible",
		"":           "pending",
		"UNKNOWN":    "pending",
	}
	for input, want := range cases {
		if got := appleRegistrationStatus(input); got != want {
			t.Fatalf("appleRegistrationStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSaveDeviceRecordPreservesAppleRegistrations(t *testing.T) {
	a := &App{data: t.TempDir()}
	registeredAt := time.Now().UTC().Add(-time.Minute)
	initial := Device{
		UDID:        "00008101-000871A22292001E",
		Product:     "iPhone13,4",
		Version:     "23G71",
		CollectedAt: time.Now().UTC().Add(-time.Hour),
		Status:      "apple_registered",
		Source:      "public_ota_gateway",
		AppleRegistrations: map[string]AppleDeviceRegistration{
			"ABCDEFGHIJ": {
				TeamID:        "ABCDEFGHIJ",
				DeviceID:      "apple-device-id",
				Status:        "registered",
				AppleStatus:   "ENABLED",
				RegisteredAt:  &registeredAt,
				LastCheckedAt: time.Now().UTC(),
			},
		},
	}
	if _, err := a.saveDeviceRecord(initial); err != nil {
		t.Fatal(err)
	}

	// A later OTA sync/re-enrollment must update collected metadata without
	// erasing the Apple-side registration that makes Ad Hoc export possible.
	refreshed := Device{
		UDID:        initial.UDID,
		Product:     initial.Product,
		Version:     initial.Version,
		CollectedAt: time.Now().UTC(),
		Status:      "pending_apple_registration",
		Source:      "public_ota_gateway",
	}
	created, err := a.saveDeviceRecord(refreshed)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("existing device was reported as newly created")
	}

	got, err := a.readDeviceRecord(initial.UDID)
	if err != nil {
		t.Fatal(err)
	}
	registration, ok := got.AppleRegistrations["ABCDEFGHIJ"]
	if !ok || registration.DeviceID != "apple-device-id" || registration.Status != "registered" {
		t.Fatalf("registration was not preserved: %#v", got.AppleRegistrations)
	}
	if got.Status != "apple_registered" {
		t.Fatalf("status = %q, want apple_registered", got.Status)
	}
}

func TestAppleDeviceDisplayNameUsesProductAndUDIDSuffix(t *testing.T) {
	d := Device{UDID: "00008101-000871A22292001E", Product: "iPhone13,4"}
	if got, want := appleDeviceDisplayName(d), "ILS iPhone13,4 2292001E"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
}
