package push

import (
	"fmt"
	"github.com/tidwall/gjson"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDynamicFeedValidation(t *testing.T) {
	for _, raw := range []string{``, `{}`, `{"code":-101}`, `{"code":0}`, `{"code":0,"data":{"items":[{}]}}`} {
		if _, err := decodeDynamicFeed([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if items, err := decodeDynamicFeed([]byte(`{"code":0,"data":{"items":[]}}`)); err != nil || len(items) != 0 {
		t.Fatal(err)
	}
}
func TestDynamicFeedPinnedOrdering(t *testing.T) {
	raw := `{"code":0,"data":{"items":[{"id_str":"1","modules":{"module_author":{"name":"old pinned","pub_ts":10}}},{"id_str":"2","modules":{"module_author":{"name":"new","pub_ts":20}}}]}}`
	items, err := decodeDynamicFeed([]byte(raw))
	if err != nil || items[0].Get("id_str").String() != "2" {
		t.Fatal(items, err)
	}
}
func TestDynamicFeedHTTP(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Cookie") != "fixture" {
			t.Error("cookie missing")
		}
		w.Header().Set("Location", "/again")
		w.WriteHeader(302)
	}))
	defer server.Close()
	if _, err := fetchDynamicFeed(newFeedClient(), server.URL, "fixture"); err == nil || calls != 1 {
		t.Fatal(fmt.Sprint(err), calls)
	}
}

func TestDynamicWatermark(t *testing.T) {
	item := func(id string, ts int64) gjson.Result {
		return gjson.Parse(fmt.Sprintf(`{"id_str":"%s","modules":{"module_author":{"pub_ts":%d}}}`, id, ts))
	}
	items := []gjson.Result{item("1", 100), item("3", 990), item("2", 980), item("3", 990)}
	p, mark := pendingDynamics(items, 0, false, 1000)
	if len(p) != 0 || mark != 990 {
		t.Fatal("baseline", p, mark)
	}
	p, mark = pendingDynamics(items, 970, true, 1000)
	if len(p) != 2 || p[0].Get("id_str").String() != "2" || mark != 990 {
		t.Fatal("new ordered/dedup", p, mark)
	}
	p, next := pendingDynamics(items, mark, true, 1000)
	if len(p) != 0 || next != mark {
		t.Fatal("replay", p, next)
	}
	p, _ = pendingDynamics(items, 50, true, 2000)
	if len(p) != 0 {
		t.Fatal("old events", p)
	}
}

// An optional public response fixture validates actual upstream schema without credentials.
func TestDynamicPublicFixture(t *testing.T) {
	path := os.Getenv("BILI_DYNAMIC_FIXTURE")
	if path == "" {
		t.Skip("no public fixture supplied")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	items, err := decodeDynamicFeed(raw)
	if err != nil || len(items) == 0 {
		t.Fatalf("public fixture: %v", err)
	}
	pending, mark := pendingDynamics(items, 0, false, items[0].Get("modules.module_author.pub_ts").Int())
	if len(pending) != 0 || mark <= 0 {
		t.Fatal("invalid baseline")
	}
}
