package webui

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/IDisposable/docker-wyze-bridge/internal/wyzeapi"
)

// Integrated-floodlight control commands (HL_CFL2). The Wyze cloud
// mirrors the light's state in property P1056 but ignores writes to it,
// so the switch has to go to the camera itself.
const (
	cmdGetFloodlightInfo uint16 = 10788
	cmdSetFloodlightInfo uint16 = 10790
)

// floodlightInfo is the envelope both commands use. Values are strings:
// switch "1" = on, "2" = off; result "1" = accepted.
type floodlightInfo struct {
	FloodlightInfo struct {
		ProtocolVersion string `json:"protocolVersion,omitempty"`
		Switch          string `json:"switch,omitempty"`
		Result          string `json:"result,omitempty"`
	} `json:"floodlightInfo"`
}

// handleFloodlight serves /api/cameras/{name}/floodlight.
// GET returns {"on":bool}; POST takes {"on":bool} and returns the same.
func (s *Server) handleFloodlight(w http.ResponseWriter, r *http.Request, info wyzeapi.CameraInfo) {
	if !info.HasFloodlight() {
		writeJSONError(w, http.StatusBadRequest, "camera has no floodlight")
		return
	}
	api := s.go2rtc()
	if api == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "go2rtc not ready")
		return
	}
	// The command session is independent of the video path, so always
	// build a wyze:// URL even when the camera streams over WebRTC.
	camURL := info.StreamURL("sd")

	switch r.Method {
	case http.MethodGet:
		raw, err := api.WyzeCommand(r.Context(), camURL, cmdGetFloodlightInfo, "")
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		var resp floodlightInfo
		if err := json.Unmarshal([]byte(raw), &resp); err != nil || resp.FloodlightInfo.Switch == "" {
			writeJSONError(w, http.StatusBadGateway, "camera did not report floodlight state")
			return
		}
		writeJSON(w, map[string]interface{}{"on": resp.FloodlightInfo.Switch == "1"})

	case http.MethodPost:
		var body struct {
			On *bool `json:"on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.On == nil {
			writeJSONError(w, http.StatusBadRequest, `body must be {"on":true|false}`)
			return
		}
		// protocolVersion is mandatory: without it the camera answers
		// result "2" and leaves the light as it was.
		var cmd floodlightInfo
		cmd.FloodlightInfo.ProtocolVersion = "1"
		cmd.FloodlightInfo.Switch = "2"
		if *body.On {
			cmd.FloodlightInfo.Switch = "1"
		}
		payload, _ := json.Marshal(cmd)

		raw, err := api.WyzeCommand(r.Context(), camURL, cmdSetFloodlightInfo, string(payload))
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		var resp floodlightInfo
		if err := json.Unmarshal([]byte(raw), &resp); err != nil || resp.FloodlightInfo.Result != "1" {
			writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("camera rejected floodlight command: %s", raw))
			return
		}
		writeJSON(w, map[string]interface{}{"status": "ok", "on": *body.On})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}
