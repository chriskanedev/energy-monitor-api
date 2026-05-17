package shelly

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const capturedShellyStatus = `{"wifi_sta":{"connected":true,"ssid":"Hyperoptic Fibre A913 2.4G","ip":"192.168.1.173","rssi":-34},"cloud":{"enabled":false,"connected":false},"mqtt":{"connected":false},"time":"21:31","unixtime":1779049903,"serial":3970,"has_update":false,"mac":"48E729687DE4","cfg_changed_cnt":5,"actions_stats":{"skipped":0},"relays":[{"ison":false,"has_timer":false,"timer_started":0,"timer_duration":0,"timer_remaining":0,"overpower":false,"is_valid":true,"source":"input"}],"emeters":[{"power":2565.42,"reactive":-811.56,"pf":-0.95,"voltage":238.59,"is_valid":true,"total":1421.8,"total_returned":150.3},{"power":0.00,"reactive":0.00,"pf":0.00,"voltage":238.59,"is_valid":true,"total":0.0,"total_returned":0.0}],"update":{"status":"idle","has_update":false,"new_version":"20230913-114150/v1.14.0-gcb84623","old_version":"20230913-114150/v1.14.0-gcb84623","beta_version":"20231107-164916/v1.14.1-rc1-g0617c15"},"ram_total":51064,"ram_free":32176,"fs_size":233681,"fs_free":157879,"uptime":13690}`

func TestFetchStatusParsesShellyEMPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(capturedShellyStatus))
	}))
	defer server.Close()

	status, err := NewClient(server.Client()).FetchStatus(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchStatus() error = %v", err)
	}
	if len(status.EMeters) != 2 {
		t.Fatalf("len(EMeters) = %d", len(status.EMeters))
	}
	if status.EMeters[0].Power != 2565.42 {
		t.Fatalf("emeter[0].Power = %f", status.EMeters[0].Power)
	}
	if status.EMeters[0].TotalReturned != 150.3 {
		t.Fatalf("emeter[0].TotalReturned = %f", status.EMeters[0].TotalReturned)
	}
}

func TestStatusURLAddsSchemeAndStatusPath(t *testing.T) {
	got, err := statusURL("192.168.1.173")
	if err != nil {
		t.Fatalf("statusURL() error = %v", err)
	}
	if got != "http://192.168.1.173/status" {
		t.Fatalf("statusURL() = %q", got)
	}
}
