package main

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

// These tests use temporary files only and never access the production
// Lightsail, iOS devices, signing keys or ILS sync token.
func TestPendingDeviceAuthAndPersistenceAcrossGatewayReopen(t *testing.T) {
    data := t.TempDir()
    dir := filepath.Join(data, "devices")
    if err := os.MkdirAll(dir, 0700); err != nil { t.Fatal(err) }

    id := strings.Repeat("a", 64)
    device := pendingDevice{
        ID: id,
        UDID: "00000000-0000000000000000",
        Name: "Test iPhone",
        Product: "iPhone",
        Version: "18.0",
        CollectedAt: time.Now().UTC(),
    }
    raw, err := json.Marshal(device)
    if err != nil { t.Fatal(err) }
    file := filepath.Join(dir, id+".json")
    if err = os.WriteFile(file, raw, 0600); err != nil { t.Fatal(err) }

    token := strings.Repeat("t", 64)
    g := &gateway{data:data, token:token}
    pending := func(bearer string) *httptest.ResponseRecorder {
        r:=httptest.NewRequest(http.MethodGet,"/api/ils/devices/pending",nil)
        if bearer != "" { r.Header.Set("Authorization","Bearer "+bearer) }
        w:=httptest.NewRecorder()
        g.pending(w,r)
        return w
    }
    if got:=pending(""); got.Code!=http.StatusUnauthorized || strings.Contains(got.Body.String(),device.UDID) {
        t.Fatalf("unauthorized pending leaked data or unexpected status: %d",got.Code)
    }
    if got:=pending("invalid");got.Code!=http.StatusUnauthorized { t.Fatalf("invalid token status: %d",got.Code) }

    // New gateway object models a service restart: persisted files remain.
    g = &gateway{data:data,token:token}
    got:=pending(token)
    if got.Code!=http.StatusOK { t.Fatalf("authorized pending status: %d",got.Code) }
    var observed []pendingDevice
    if err:=json.Unmarshal(got.Body.Bytes(),&observed);err!=nil { t.Fatal(err) }
    if len(observed)!=1 || observed[0].ID!=id || observed[0].UDID!=device.UDID {
        t.Fatalf("pending device persistence failed (count=%d)",len(observed))
    }

    // Authentication failure must not ACK/delete a device.
    denied:=httptest.NewRecorder()
    bad:=httptest.NewRequest(http.MethodPost,"/api/ils/devices/"+id+"/ack",nil)
    bad.SetPathValue("id",id)
    g.ack(denied,bad)
    if denied.Code!=http.StatusUnauthorized { t.Fatalf("unauthorized ack status: %d",denied.Code) }
    if _,err:=os.Stat(file);err!=nil { t.Fatalf("unauthorized ack changed persisted data: %v",err) }

    accepted:=httptest.NewRecorder()
    good:=httptest.NewRequest(http.MethodPost,"/api/ils/devices/"+id+"/ack",nil)
    good.Header.Set("Authorization","Bearer "+token)
    good.SetPathValue("id",id)
    g.ack(accepted,good)
    if accepted.Code!=http.StatusNoContent { t.Fatalf("ack status: %d",accepted.Code) }
    if _,err:=os.Stat(file);!os.IsNotExist(err) { t.Fatalf("ack did not consume pending device: %v",err) }
}
