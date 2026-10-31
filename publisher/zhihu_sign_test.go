package publisher

import (
	"net/http"
	"testing"
)

func TestZhihuSignRequestUsesExactRequestURI(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://www.zhihu.com/api/v4/articles/my_drafts?offset=0&limit=10&include=data%5B*%5D.schedule", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := zhihuSignRequest(req, "test-dc0"); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("x-zse-93"); got != "101_3_3.0" {
		t.Fatalf("x-zse-93 = %q", got)
	}
	const expected = "2.0_M0OK7CyA+vsiKFF3wiYOoAw9mF4qgJKSD=i6m2avwMe9K2D+phuCjPWZY8cWZzVM"
	if got := req.Header.Get("x-zse-96"); got != expected {
		t.Fatalf("x-zse-96 = %q, want %q", got, expected)
	}
}

func TestZhihuDC0FromSessionFallsBackToCapturedHeaders(t *testing.T) {
	session := Session{
		Cookies:             []BrowserCookie{{Name: "z_c0", Value: "login", Domain: ".zhihu.com"}},
		RequestCookieHeader: "z_c0=login; d_c0=header-dc0; _xsrf=csrf",
	}
	if got := zhihuDC0FromSession(session); got != "header-dc0" {
		t.Fatalf("d_c0 = %q", got)
	}
}

func TestZhihuSignRequestRequiresDC0ForWWWAPI(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://www.zhihu.com/api/v4/articles/my_drafts", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := zhihuSignRequest(req, ""); err == nil {
		t.Fatal("expected missing d_c0 error")
	}
}

func TestZhihuSignRequestSkipsZhuanlanEndpoints(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://zhuanlan.zhihu.com/api/articles/drafts", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := zhihuSignRequest(req, ""); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("x-zse-96") != "" {
		t.Fatal("zhuanlan request should not be signed")
	}
}
