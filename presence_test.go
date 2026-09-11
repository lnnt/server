package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// presenceWatcher 连接一个 SSE 流,持续解析 presence 事件,
// 把最新一次看到的用户列表原子保存下来。
type presenceWatcher struct {
	mu      sync.Mutex
	latest  []UserEntry
	sawData bool
}

func (w *presenceWatcher) set(es []UserEntry) {
	w.mu.Lock()
	w.latest = es
	w.sawData = true
	w.mu.Unlock()
}

func (w *presenceWatcher) get() ([]UserEntry, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]UserEntry, len(w.latest))
	copy(cp, w.latest)
	return cp, w.sawData
}

// watch 启动一个 goroutine 读取 SSE,presence 事件更新 w。返回关闭函数。
func watch(t *testing.T, baseURL, roomid, nick string) (*presenceWatcher, func()) {
	t.Helper()
	url := baseURL + "/stream/" + roomid
	if nick != "" {
		url += "?nick=" + nick
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect status = %d", resp.StatusCode)
	}
	w := &presenceWatcher{}
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var event string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				if event != "presence" {
					continue
				}
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				var es []UserEntry
				if json.Unmarshal([]byte(data), &es) == nil {
					w.set(es)
				}
			}
		}
	}()
	return w, func() { resp.Body.Close() }
}

func waitFor(t *testing.T, w *presenceWatcher, want map[string]int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		es, ok := w.get()
		if ok && matches(es, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	es, _ := w.get()
	t.Fatalf("presence never reached %v (last saw %v)", want, es)
}

func matches(es []UserEntry, want map[string]int) bool {
	got := map[string]int{}
	for _, e := range es {
		got[e.Nick] = e.Count
	}
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

func newTestServer() (*httptest.Server, func()) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/stream/:roomid", streamRoom)
	srv := httptest.NewServer(r)
	return srv, srv.Close
}

func TestPresenceLifecycle(t *testing.T) {
	srv, srvClose := newTestServer()
	defer srvClose()

	obs, obsClose := watch(t, srv.URL, "r1", "observer")
	defer obsClose()
	waitFor(t, obs, map[string]int{"observer": 1}, 2*time.Second)

	alice, aliceClose := watch(t, srv.URL, "r1", "alice")
	defer aliceClose()
	waitFor(t, obs, map[string]int{"observer": 1, "alice": 1}, 2*time.Second)
	// 新连接者应能立即看到含自己在内的列表
	waitFor(t, alice, map[string]int{"observer": 1, "alice": 1}, 2*time.Second)

	// 同名开第二个标签页 → alice 计数变 2
	_, alice2Close := watch(t, srv.URL, "r1", "alice")
	waitFor(t, obs, map[string]int{"observer": 1, "alice": 2}, 2*time.Second)

	// 关掉一个标签页 → 计数回到 1
	alice2Close()
	waitFor(t, obs, map[string]int{"observer": 1, "alice": 1}, 2*time.Second)

	// alice 全部断开 → 列表只剩 observer(关键:验证不会卡死)
	aliceClose()
	waitFor(t, obs, map[string]int{"observer": 1}, 3*time.Second)
}

func TestPresenceRoomIsolation(t *testing.T) {
	srv, srvClose := newTestServer()
	defer srvClose()

	a, aClose := watch(t, srv.URL, "roomA", "alice")
	defer aClose()
	b, bClose := watch(t, srv.URL, "roomB", "bob")
	defer bClose()

	waitFor(t, a, map[string]int{"alice": 1}, 2*time.Second)
	waitFor(t, b, map[string]int{"bob": 1}, 2*time.Second)
}

func TestPresenceAnonymous(t *testing.T) {
	srv, srvClose := newTestServer()
	defer srvClose()

	a, aClose := watch(t, srv.URL, "r3", "")
	defer aClose()
	waitFor(t, a, map[string]int{"(anonymous)": 1}, 2*time.Second)
}
