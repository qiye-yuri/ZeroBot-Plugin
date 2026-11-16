package localadmin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type costRoundTrip func(*http.Request) (*http.Response, error)

func (f costRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestImageCostsPaginationExactAmountsAndScope(t *testing.T) {
	start := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	calls := 0
	client := &http.Client{Transport: costRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		if r.URL.Host != "api.openai.com" || q.Get("project_ids") != "proj_test" || q.Get("api_key_ids") != "key_test" || r.Header.Get("Authorization") != "Bearer admin-test" {
			t.Fatal("scope/auth")
		}
		amount := "0.008192"
		item := "gpt-image-2.5-sunburst image, input"
		if calls == 2 {
			amount = "0.105690"
			item = "gpt-image-2.5-sunburst image, output"
			if q.Get("page") != "next" {
				t.Fatal("pagination")
			}
		}
		b, _ := json.Marshal(map[string]any{"has_more": calls == 1, "next_page": "next", "data": []any{map[string]any{"start_time": start.Unix(), "end_time": end.Unix(), "results": []any{map[string]any{"line_item": item, "project_id": "proj_test", "amount": map[string]any{"value": json.Number(amount), "currency": "usd"}, "quantity": 1024, "quantity_unit": "tokens"}}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}
	report, e := fetchImageCosts(context.Background(), client, imageCostConfig{Key: "admin-test", ProjectID: "proj_test", APIKeyID: "key_test"}, start, end)
	if e != nil || report.TotalUSD != "0.113882000" || calls != 2 {
		t.Fatal(report, e, calls)
	}
}
func TestImageCostsRejectErrorsWithoutLeakingSecrets(t *testing.T) {
	client := &http.Client{Transport: costRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("admin-test"))}, nil
	})}
	_, e := fetchImageCosts(context.Background(), client, imageCostConfig{Key: "admin-test", ProjectID: "proj_test"}, time.Now().Add(-24*time.Hour), time.Now())
	if e == nil || strings.Contains(e.Error(), "admin-test") || !strings.Contains(e.Error(), "403") {
		t.Fatal(e)
	}
}
func TestImageCostsConfigAuthSecretPersistenceAndRevision(t *testing.T) {
	s := newTestServer(t)
	expectStatus(t, request(s, "GET", "/api/ai-image-costs", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	initial := request(s, "GET", "/api/ai-image-costs", nil, cookie, "")
	expectStatus(t, initial, 200)
	var v imageCostView
	if e := json.Unmarshal(initial.Body.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	body := map[string]any{"revision": v.Revision, "projectID": "proj_test", "proxyURL": "http://127.0.0.1:7897", "key": map[string]any{"action": "replace", "value": "admin-test"}}
	expectStatus(t, request(s, "POST", "/api/ai-image-costs", body, cookie, "bad"), 403)
	saved := request(s, "POST", "/api/ai-image-costs", body, cookie, csrf)
	expectStatus(t, saved, 200)
	if strings.Contains(saved.Body.String(), "admin-test") {
		t.Fatal("secret leaked")
	}
	expectStatus(t, request(s, "POST", "/api/ai-image-costs", body, cookie, csrf), 409)
	db, e := s.imageCostDB()
	if e != nil {
		t.Fatal(e)
	}
	c, _, e := readImageCostConfig(db)
	db.Close()
	if e != nil || c.Key != "admin-test" || c.ProjectID != "proj_test" {
		t.Fatal("not persisted", e)
	}
	expectStatus(t, request(s, "POST", "/api/ai-image-costs/query", map[string]any{}, cookie, "bad"), 403)
	expectStatus(t, request(s, "POST", "/api/ai-image-costs/query", map[string]any{"start": "2026-01-01", "end": "2026-10-30"}, cookie, csrf), 400)
}
