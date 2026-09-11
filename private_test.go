package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// chatWatcher 连接一个 SSE 流,收集 message 事件。
type chatWatcher struct {
	mu   sync.Mutex
	msgs []map[string]any
}

func (w *chatWatcher) add(m map[string]any) {
	w.mu.Lock()
	w.msgs = append(w.msgs, m)
	w.mu.Unlock()
}

func (w *chatWatcher) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.msgs)
}

func (w *chatWatcher) snapshot() []map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]map[string]any(nil), w.msgs...)
}

func newChatTestServer() *httptest.Server {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/stream/:roomid", streamRoom)
	r.POST("/room-post/:roomid", roomPOST)
	return httptest.NewServer(r)
}

// connectChat 建立 SSE 连接,返回消息收集器和关闭函数。
func connectChat(t *testing.T, baseURL, roomid, nick string) (*chatWatcher, func()) {
	t.Helper()
	u := baseURL + "/stream/" + roomid
	if nick != "" {
		u += "?nick=" + url.QueryEscape(nick)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect status = %d", resp.StatusCode)
	}
	w := &chatWatcher{}
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var event string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:") && event == "message":
				var m map[string]any
				if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &m) == nil {
					w.add(m)
				}
			}
		}
	}()
	return w, func() { resp.Body.Close() }
}

// waitOnline 轮询房间在线表,直到 want 中的用户全部出现。
func waitOnline(t *testing.T, roomid string, want ...string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r := room(roomid)
		r.mu.Lock()
		ok := true
		for _, nick := range want {
			if r.users[nick] == 0 {
				ok = false
				break
			}
		}
		r.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("users %v never came online in room %s", want, roomid)
}

func waitForCount(t *testing.T, w *chatWatcher, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if w.count() >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("watcher saw %d messages, want %d (last %v)", w.count(), want, w.snapshot())
}

func postMessage(t *testing.T, baseURL, roomid, nick, message string) *http.Response {
	t.Helper()
	form := url.Values{"message": []string{message}}
	resp, err := http.PostForm(baseURL+"/room-post/"+roomid+"?nick="+url.QueryEscape(nick), form)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

func TestPrivateMessageOnlyReachesMentionedUser(t *testing.T) {
	srv := newChatTestServer()
	defer srv.Close()

	alice, aliceClose := connectChat(t, srv.URL, "dm1", "alice")
	defer aliceClose()
	bob, bobClose := connectChat(t, srv.URL, "dm1", "bob")
	defer bobClose()
	carol, carolClose := connectChat(t, srv.URL, "dm1", "carol")
	defer carolClose()

	waitOnline(t, "dm1", "alice", "bob", "carol")

	resp := postMessage(t, srv.URL, "dm1", "alice", "@bob hello, just between us")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// bob(收件人)和 alice(发件人自己)各收到一条;carol 收不到。
	waitForCount(t, bob, 1, 2*time.Second)
	waitForCount(t, alice, 1, 2*time.Second)

	// 再广播一条公开消息作为「投递通道仍然活着」的对照。
	pub := postMessage(t, srv.URL, "dm1", "carol", "public ping")
	pub.Body.Close()
	waitForCount(t, carol, 1, 2*time.Second)
	waitForCount(t, bob, 2, 2*time.Second)
	waitForCount(t, alice, 2, 2*time.Second)

	msgs := bob.snapshot()
	if msgs[0]["private"] != true || msgs[0]["to"] != "bob" || msgs[0]["nick"] != "alice" {
		t.Fatalf("bob's private message = %v", msgs[0])
	}
	if msgs[1]["private"] == true {
		t.Fatalf("public message was flagged private: %v", msgs[1])
	}

	own := alice.snapshot()[0]
	if own["private"] != true || own["to"] != "bob" {
		t.Fatalf("alice should see her own private message, got %v", own)
	}
}

func TestPrivateMessageToOfflineUserFails(t *testing.T) {
	srv := newChatTestServer()
	defer srv.Close()

	alice, aliceClose := connectChat(t, srv.URL, "dm2", "alice")
	defer aliceClose()
	waitOnline(t, "dm2", "alice")

	resp := postMessage(t, srv.URL, "dm2", "alice", "@ghost are you there?")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	// 什么都没投出去,连发送者自己也不该看到。
	time.Sleep(200 * time.Millisecond)
	if n := alice.count(); n != 0 {
		t.Fatalf("alice received %d messages for an offline target, want 0", n)
	}
}

func TestBareMentionIsNotPrivate(t *testing.T) {
	srv := newChatTestServer()
	defer srv.Close()

	alice, aliceClose := connectChat(t, srv.URL, "dm3", "alice")
	defer aliceClose()
	bob, bobClose := connectChat(t, srv.URL, "dm3", "bob")
	defer bobClose()
	waitOnline(t, "dm3", "alice", "bob")

	// "@bob"(只有提及没有正文)按普通消息广播。
	resp := postMessage(t, srv.URL, "dm3", "alice", "@bob")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	waitForCount(t, bob, 1, 2*time.Second)
	waitForCount(t, alice, 1, 2*time.Second)
	if alice.snapshot()[0]["private"] == true {
		t.Fatal("bare mention should not be private")
	}
}

// 带空格、以及被截断成 "xxx..." 的昵称,都要能匹配到在线用户。
func TestMentionMatchesSpacedAndTruncatedNicks(t *testing.T) {
	online := []string{"bob", "bob cat", "abcdefghijkl..."}
	cases := []struct {
		msg     string
		to      string
		isDM    bool
		unknown string
	}{
		{"@bob hi", "bob", true, ""},
		{"@bob cat hi", "bob cat", true, ""},                  // 最长匹配,不落到 bob
		{"@abcdefghijklmnop hi", "abcdefghijkl...", true, ""}, // 打全名,匹配截断后的昵称
		{"@ghost hi", "", false, "ghost"},
		{"@bob", "", false, ""},  // 无正文 → 普通消息
		{"@x hi", "", false, ""}, // 昵称至少 2 字符 → 普通消息
		{"plain hi", "", false, ""},
	}
	for _, c := range cases {
		to, isDM, unknown := parseMention(c.msg, online)
		if to != c.to || isDM != c.isDM || unknown != c.unknown {
			t.Errorf("parseMention(%q) = (%q, %v, %q), want (%q, %v, %q)",
				c.msg, to, isDM, unknown, c.to, c.isDM, c.unknown)
		}
	}
}

func TestPrivateMessageToSpacedNick(t *testing.T) {
	srv := newChatTestServer()
	defer srv.Close()

	_, aliceClose := connectChat(t, srv.URL, "dm4", "alice")
	defer aliceClose()
	bob, bobClose := connectChat(t, srv.URL, "dm4", "bob cat")
	defer bobClose()
	bobcat, bobcatClose := connectChat(t, srv.URL, "dm4", "bob")
	defer bobcatClose()
	waitOnline(t, "dm4", "alice", "bob", "bob cat")

	resp := postMessage(t, srv.URL, "dm4", "alice", "@bob cat meet at the pier")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	// 「bob cat」才是目标;同房间的单名 bob 不该收到。
	waitForCount(t, bob, 1, 2*time.Second)
	if got := bob.snapshot()[0]; got["to"] != "bob cat" {
		t.Fatalf("bob cat got %v", got)
	}
	time.Sleep(200 * time.Millisecond)
	if n := bobcat.count(); n != 0 {
		t.Fatalf("user %q leaked the spaced-nick DM: %v", "bob", bobcat.snapshot())
	}
}
