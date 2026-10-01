package utils

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// pendoTrackEvent represents a server-side Pendo track event payload.
type pendoTrackEvent struct {
	Type       string         `json:"type"`
	Event      string         `json:"event"`
	VisitorID  string         `json:"visitorId"`
	AccountID  string         `json:"accountId"`
	Timestamp  int64          `json:"timestamp"`
	Properties map[string]any `json:"properties,omitempty"`
}

var pendoHTTPClient = &http.Client{Timeout: 5 * time.Second}

// PendoTrack sends a server-side track event to the Pendo Track API.
// The HTTP call runs in a goroutine so it never blocks the caller.
// Errors are logged but never propagate to application logic.
//
// TODO: Set PENDO_INTEGRATION_KEY environment variable with the Pendo
// server-side integration key (x-pendo-integration-key) for events to be sent.
// Optionally set PENDO_DATA_HOST to override the default Pendo data host
// (defaults to https://data.pendo.io).
func PendoTrack(eventName string, visitorID string, accountID string, properties map[string]any) {
	go func() {
		integrationKey := os.Getenv("PENDO_INTEGRATION_KEY")
		if integrationKey == "" {
			return
		}

		payload := pendoTrackEvent{
			Type:       "track",
			Event:      eventName,
			VisitorID:  visitorID,
			AccountID:  accountID,
			Timestamp:  time.Now().UnixMilli(),
			Properties: properties,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			slog.Error("pendo: failed to marshal track event", slog.String("event", eventName), slog.String("error", err.Error()))
			return
		}

		dataHost := os.Getenv("PENDO_DATA_HOST")
		if dataHost == "" {
			dataHost = "https://data.pendo.io"
		}

		req, err := http.NewRequest(http.MethodPost, dataHost+"/data/track", bytes.NewReader(body))
		if err != nil {
			slog.Error("pendo: failed to create track request", slog.String("event", eventName), slog.String("error", err.Error()))
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-pendo-integration-key", integrationKey)

		resp, err := pendoHTTPClient.Do(req)
		if err != nil {
			slog.Error("pendo: failed to send track event", slog.String("event", eventName), slog.String("error", err.Error()))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			slog.Error("pendo: track event returned error status", slog.String("event", eventName), slog.Int("status", resp.StatusCode))
		}
	}()
}
