package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"bottrade/internal/auth"
)

func TestAccessGating(t *testing.T) {
	cfg := testConfigWith(t, map[string]string{
		"ACCESS_OPEN":            "false",
		"TELEGRAM_ADMIN_USER_ID": "468848033",
	})
	tk, _ := auth.NewTokenizer(bytes.Repeat([]byte("k"), auth.MinSecretSize), 0)
	adminTok, _ := tk.Issue("tg:468848033", "admin", "user")
	userTok, _ := tk.Issue("tg:999", "newbie", "user")
	server := NewServer(cfg, nil, testLogger(), WithTokenizer(tk))

	call := func(method, path, tok string, body any) (int, map[string]any) {
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := server.App().Test(req)
		if err != nil {
			t.Fatalf("Test %s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	// New user: not approved.
	if _, me := call(http.MethodGet, "/api/me", userTok, nil); me["approved"] != false || me["admin"] != false {
		t.Fatalf("new user me = %v, want approved/admin false", me)
	}
	// Admin: always approved.
	if _, me := call(http.MethodGet, "/api/me", adminTok, nil); me["approved"] != true || me["admin"] != true {
		t.Fatalf("admin me = %v, want approved+admin true", me)
	}
	// User requests access.
	if _, out := call(http.MethodPost, "/api/access/request", userTok, nil); out["status"] != "requested" {
		t.Fatalf("request = %v, want requested", out)
	}
	if _, me := call(http.MethodGet, "/api/me", userTok, nil); me["status"] != "requested" {
		t.Fatalf("me status = %v, want requested", me)
	}
	// Non-admin cannot view or approve.
	if code, _ := call(http.MethodGet, "/api/admin/pending", userTok, nil); code != http.StatusForbidden {
		t.Fatalf("non-admin pending = %d, want 403", code)
	}
	// Admin sees the request and approves it.
	_, pend := call(http.MethodGet, "/api/admin/pending", adminTok, nil)
	list, _ := pend["pending"].([]any)
	if len(list) != 1 {
		t.Fatalf("pending = %v, want 1", pend)
	}
	if code, _ := call(http.MethodPost, "/api/admin/approve", adminTok, map[string]any{"subject": "tg:999"}); code != http.StatusOK {
		t.Fatalf("approve status = %d", code)
	}
	// User is now approved.
	if _, me := call(http.MethodGet, "/api/me", userTok, nil); me["approved"] != true {
		t.Fatalf("after approve, me = %v, want approved true", me)
	}
}
