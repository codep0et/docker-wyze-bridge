package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/IDisposable/docker-wyze-bridge/internal/camera"
	"github.com/IDisposable/docker-wyze-bridge/internal/go2rtcmgr"
	"github.com/IDisposable/docker-wyze-bridge/internal/wyzeapi"
)

// wyzeCommandCall is one request the fake go2rtc received.
type wyzeCommandCall struct {
	URL     string `json:"url"`
	Code    uint16 `json:"code"`
	Payload string `json:"payload"`
}

// floodlightServer wires a Server to a fake go2rtc whose
// /api/wyze/command answers with reply, and registers a floodlight cam.
func floodlightServer(t *testing.T, model, reply string) (*Server, *[]wyzeCommandCall) {
	t.Helper()
	srv, _ := testServer(t)

	var calls []wyzeCommandCall
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/wyze/command" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var c wyzeCommandCall
		_ = json.NewDecoder(r.Body).Decode(&c)
		calls = append(calls, c)
		_ = json.NewEncoder(w).Encode(map[string]string{"payload": reply})
	}))
	t.Cleanup(fake.Close)
	srv.SetGo2RTCAPI(go2rtcmgr.NewAPIClient(fake.URL, zerolog.Nop()))

	cam := camera.NewCamera(wyzeapi.CameraInfo{
		Name: "yard", Model: model, MAC: "AABBCCDDEEFF", LanIP: "10.0.0.9", P2PID: "UID", ENR: "enr", DTLS: true,
	}, "sd", true, false)
	srv.camMgr.InjectCamera("yard", cam)
	return srv, &calls
}

func TestFloodlight_SetOnSendsSwitchWithProtocolVersion(t *testing.T) {
	srv, calls := floodlightServer(t, "HL_CFL2", `{"floodlightInfo":{"protocolVersion":"1","result":"1"}}`)

	req := httptest.NewRequest("POST", "/api/cameras/yard/floodlight", bytes.NewBufferString(`{"on":true}`))
	w := httptest.NewRecorder()
	srv.handleAPICameraAction(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if len(*calls) != 1 {
		t.Fatalf("go2rtc calls = %d, want 1", len(*calls))
	}
	c := (*calls)[0]
	if c.Code != 10790 {
		t.Errorf("code = %d, want 10790", c.Code)
	}
	if !strings.HasPrefix(c.URL, "wyze://10.0.0.9?") {
		t.Errorf("url = %q, want a wyze:// URL for the camera", c.URL)
	}
	var sent floodlightInfo
	if err := json.Unmarshal([]byte(c.Payload), &sent); err != nil {
		t.Fatalf("payload not JSON: %q", c.Payload)
	}
	if sent.FloodlightInfo.Switch != "1" || sent.FloodlightInfo.ProtocolVersion != "1" {
		t.Errorf("payload = %q, want switch 1 with protocolVersion 1", c.Payload)
	}
}

func TestFloodlight_SetOffSendsSwitch2(t *testing.T) {
	srv, calls := floodlightServer(t, "HL_CFL2", `{"floodlightInfo":{"protocolVersion":"1","result":"1"}}`)

	req := httptest.NewRequest("POST", "/api/cameras/yard/floodlight", bytes.NewBufferString(`{"on":false}`))
	w := httptest.NewRecorder()
	srv.handleAPICameraAction(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var sent floodlightInfo
	_ = json.Unmarshal([]byte((*calls)[0].Payload), &sent)
	if sent.FloodlightInfo.Switch != "2" {
		t.Errorf("switch = %q, want 2", sent.FloodlightInfo.Switch)
	}
}

func TestFloodlight_CameraRejectionIsAnError(t *testing.T) {
	srv, _ := floodlightServer(t, "HL_CFL2", `{"floodlightInfo":{"protocolVersion":"1","result":"2"}}`)

	req := httptest.NewRequest("POST", "/api/cameras/yard/floodlight", bytes.NewBufferString(`{"on":true}`))
	w := httptest.NewRecorder()
	srv.handleAPICameraAction(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 when the camera answers result 2", w.Code)
	}
}

func TestFloodlight_GetReportsState(t *testing.T) {
	for reply, want := range map[string]bool{
		`{"floodlightInfo":{"protocolVersion":"1","switch":"1","brightness":"254"}}`: true,
		`{"floodlightInfo":{"protocolVersion":"1","switch":"2","brightness":"254"}}`: false,
	} {
		srv, calls := floodlightServer(t, "HL_CFL2", reply)

		req := httptest.NewRequest("GET", "/api/cameras/yard/floodlight", nil)
		w := httptest.NewRecorder()
		srv.handleAPICameraAction(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
		}
		var resp map[string]bool
		_ = json.NewDecoder(w.Body).Decode(&resp)
		if resp["on"] != want {
			t.Errorf("on = %v, want %v for %s", resp["on"], want, reply)
		}
		if (*calls)[0].Code != 10788 {
			t.Errorf("code = %d, want 10788", (*calls)[0].Code)
		}
	}
}

func TestFloodlight_RejectsCameraWithoutFloodlight(t *testing.T) {
	srv, calls := floodlightServer(t, "WYZE_CAKP2JFUS", `{}`)

	req := httptest.NewRequest("POST", "/api/cameras/yard/floodlight", bytes.NewBufferString(`{"on":true}`))
	w := httptest.NewRecorder()
	srv.handleAPICameraAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if len(*calls) != 0 {
		t.Errorf("go2rtc was called %d times for a camera with no floodlight", len(*calls))
	}
}

func TestFloodlight_RejectsBodyWithoutOn(t *testing.T) {
	srv, calls := floodlightServer(t, "HL_CFL2", `{}`)

	req := httptest.NewRequest("POST", "/api/cameras/yard/floodlight", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	srv.handleAPICameraAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if len(*calls) != 0 {
		t.Errorf("go2rtc was called %d times for an invalid body", len(*calls))
	}
}
