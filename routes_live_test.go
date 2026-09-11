package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newFullServer mirrors main.go's router setup so we can probe every route
// (and the private-message plumbing end to end) without a live port.
func newFullServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rateLimit, gin.Recovery())
	r.LoadHTMLGlob("resources/*.templ.html")
	r.Static("/static", "resources/static")
	r.GET("/", index)
	r.GET("/room/:roomid", roomGET)
	r.POST("/room-post/:roomid", roomPOST)
	r.GET("/stream/:roomid", streamRoom)
	return httptest.NewServer(r)
}

func TestFullRoutes(t *testing.T) {
	srv := newFullServer(t)
	defer srv.Close()

	get := func(path string) (int, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	if code, _ := get("/room/hn?nick=alice"); code != http.StatusOK {
		t.Fatalf("GET /room/hn code = %d", code)
	}
	if code, _ := get("/static/realtime.js"); code != http.StatusOK {
		t.Fatalf("GET /static/realtime.js code = %d", code)
	}
	// "/" 301 → "/room/hn",http.Client 默认跟随重定向,最终应为 200。
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/room/hn" {
		t.Fatalf("GET / ended at %s with code %d", resp.Request.URL.Path, resp.StatusCode)
	}
}

// TestLivePrivateMessageFlow connects three SSE clients, sends a DM from alice
// to bob, and asserts exactly alice+bob (each tab) see it and carol never does.
func TestLivePrivateMessageFlow(t *testing.T) {
	srv := newFullServer(t)
	defer srv.Close()

	alice, aliceClose := connectChat(t, srv.URL, "live", "alice")
	defer aliceClose()
	alice2, alice2Close := connectChat(t, srv.URL, "live", "alice")
	defer alice2Close()
	bob, bobClose := connectChat(t, srv.URL, "live", "bob")
	defer bobClose()
	carol, carolClose := connectChat(t, srv.URL, "live", "carol")
	defer carolClose()

	waitOnline(t, "live", "alice", "bob", "carol")

	form := url.Values{"message": []string{"@bob meet me at 8"}}
	resp, err := http.PostForm(srv.URL+"/room-post/live?nick=alice", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DM post: %d %s", resp.StatusCode, body)
	}
	t.Logf("DM post body: %s", body)

	waitForCount(t, bob, 1, 2*time.Second)
	waitForCount(t, alice, 1, 2*time.Second)
	waitForCount(t, alice2, 1, 2*time.Second) // 同名第二个标签页也收到
	time.Sleep(300 * time.Millisecond)
	if n := carol.count(); n != 0 {
		t.Fatalf("carol leaked private message: %v", carol.snapshot())
	}

	// 公开消息仍然广播。
	pub, err := http.PostForm(srv.URL+"/room-post/live?nick=carol", url.Values{"message": []string{"hi everyone"}})
	if err != nil {
		t.Fatal(err)
	}
	pub.Body.Close()
	waitForCount(t, bob, 2, 2*time.Second)
	waitForCount(t, alice, 2, 2*time.Second)
	waitForCount(t, carol, 1, 2*time.Second)
}
