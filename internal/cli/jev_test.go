package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type jevStatusReport struct {
	APIKey    bool   `json:"api_key"`
	Reachable bool   `json:"reachable"`
	Model     string `json:"model"`
	Endpoint  string `json:"endpoint"`
	Trouble   string `json:"trouble,omitempty"`
}

func TestJevStatusWithNoKeySendsNoRequest(t *testing.T) {
	repo := t.TempDir()
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENRECORD_JEV_ENDPOINT", server.URL)

	code, stdout, _ := runIn(t, repo, "jev", "status")
	if code == exitOK {
		t.Error("jev status with no key exited 0")
	}
	report := decode[jevStatusReport](t, stdout)
	if report.APIKey {
		t.Error("api_key = true with no key set")
	}
	if report.Reachable {
		t.Error("reachable = true with no key set")
	}
	if called {
		t.Error("a request was sent to the endpoint despite no key being set")
	}
}

func TestJevStatusHealthy(t *testing.T) {
	repo := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Questions map[string]any `json:"questions"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		answers := make(map[string]map[string]float64, len(body.Questions))
		for name := range body.Questions {
			answers[name] = map[string]float64{"noul": 0.1}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "a-key")
	t.Setenv("OPENRECORD_JEV_ENDPOINT", server.URL)

	code, stdout, stderr := runIn(t, repo, "jev", "status")
	if code != exitOK {
		t.Fatalf("jev status failed: exit %d (%s)", code, stderr)
	}
	report := decode[jevStatusReport](t, stdout)
	if !report.APIKey || !report.Reachable {
		t.Errorf("report = %+v, want api_key and reachable both true", report)
	}
}

func TestJevStatusUnreachable(t *testing.T) {
	repo := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "a-key")
	t.Setenv("OPENRECORD_JEV_ENDPOINT", server.URL)

	code, stdout, _ := runIn(t, repo, "jev", "status")
	if code == exitOK {
		t.Error("jev status with an unreachable endpoint exited 0")
	}
	report := decode[jevStatusReport](t, stdout)
	if !report.APIKey {
		t.Error("api_key = false despite a key being set")
	}
	if report.Reachable {
		t.Error("reachable = true despite the endpoint failing every request")
	}
	if report.Trouble == "" {
		t.Error("trouble is empty for an unreachable endpoint")
	}
}
