package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestRegisterServesEmbeddedIndex(t *testing.T) {
	app := fiber.New()
	if err := Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}

	body, status := get(t, app, "/")
	if status != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", status)
	}
	if !strings.Contains(body, "Trading Bot Dashboard") {
		t.Fatalf("GET / body does not contain dashboard marker: %q", body)
	}
}

func TestRegisterSPAFallbackServesIndex(t *testing.T) {
	app := fiber.New()
	if err := Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// A browser navigation (Accept: text/html) to a deep-link route with no
	// embedded file must fall back to index.html (200) so client-side routing
	// survives a refresh — including routes whose last segment contains a dot.
	for _, route := range []string{"/positions/abc123", "/positions/v1.2", "/u/john.doe"} {
		body, status := getNav(t, app, route)
		if status != http.StatusOK {
			t.Fatalf("SPA fallback %s status = %d, want 200", route, status)
		}
		if !strings.Contains(body, "Trading Bot Dashboard") {
			t.Fatalf("SPA fallback %s did not serve index.html: %q", route, body)
		}
	}
}

func TestRegisterMissingAssetReturns404(t *testing.T) {
	app := fiber.New()
	if err := Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// A path that looks like a file (has an extension) but does not exist must
	// return a real 404, not the SPA shell — otherwise broken asset references
	// and wrong-path probes silently look healthy.
	for _, target := range []string{"/assets/app.12345.js", "/missing.png", "/static/style.css"} {
		body, status := get(t, app, target)
		if status != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404 (body: %q)", target, status, body)
		}
		if strings.Contains(body, "Trading Bot Dashboard") {
			t.Fatalf("GET %s served the SPA shell instead of 404", target)
		}
	}
}

func TestRegisterDoesNotShadowEarlierRoutes(t *testing.T) {
	app := fiber.New()
	// API route registered before the dashboard catch-all, mirroring the api
	// server's registration order.
	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok") })
	if err := Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}

	body, status := get(t, app, "/healthz")
	if status != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want 200", status)
	}
	if body != "ok" {
		t.Fatalf("GET /healthz body = %q, want \"ok\" (dashboard shadowed the API route)", body)
	}
}

// get issues a plain GET (no Accept), mimicking an asset/XHR fetch.
func get(t *testing.T, app *fiber.App, target string) (string, int) {
	t.Helper()
	return getWith(t, app, target, "")
}

// getNav issues a browser-navigation GET (Accept: text/html).
func getNav(t *testing.T, app *fiber.App, target string) (string, int) {
	t.Helper()
	return getWith(t, app, target, "text/html")
}

func getWith(t *testing.T, app *fiber.App, target, accept string) (string, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test %s: %v", target, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", target, err)
	}
	return string(body), resp.StatusCode
}
