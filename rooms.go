package main

import (
	"sort"
	"sync"

	"github.com/dustin/go-broadcast"
)

// feedItem 是投递给 SSE 监听者的带类型事件,
// Kind 即 SSE 事件名("message" / "presence")。
type feedItem struct {
	Kind string
	Data any
}

// UserEntry 是在线用户列表的一项。
type UserEntry struct {
	Nick  string `json:"nick"`
	Count int    `json:"count"`
}

// listenerBuffer 是每个 SSE 监听者通道的缓冲大小。
const listenerBuffer = 64

var (
	mutexRooms   sync.Mutex
	roomChannels = make(map[string]broadcast.Broadcaster)

	mutexUsers sync.Mutex
	roomUsers  = make(map[string]map[string]int) // roomid -> nick -> 连接数
)

func openListener(roomid string) chan any {
	// 带缓冲:go-broadcast 的 run() 是单协程且向每个监听者阻塞式发送,
	// 若通道无缓冲,客户端断开后尚未注销时广播会卡死整个房间。
	listener := make(chan any, listenerBuffer)
	room(roomid).Register(listener)
	return listener
}

func closeListener(roomid string, listener chan any) {
	room(roomid).Unregister(listener)
	close(listener)
}

func submitMessage(roomid string, post any) {
	room(roomid).Submit(feedItem{Kind: "message", Data: post})
}

// addUser 记录一个在线连接,并向房间内所有监听者广播最新的用户列表。
func addUser(roomid, nick string) {
	mutexUsers.Lock()
	users, ok := roomUsers[roomid]
	if !ok {
		users = make(map[string]int)
		roomUsers[roomid] = users
	}
	users[nick]++
	entries := userEntries(users)
	mutexUsers.Unlock()

	room(roomid).Submit(feedItem{Kind: "presence", Data: entries})
}

// removeUser 在一个连接断开时调用,计数归零则从列表中移除。
func removeUser(roomid, nick string) {
	mutexUsers.Lock()
	users, ok := roomUsers[roomid]
	if !ok {
		mutexUsers.Unlock()
		return
	}
	if users[nick]--; users[nick] <= 0 {
		delete(users, nick)
	}
	entries := userEntries(users)
	mutexUsers.Unlock()

	room(roomid).Submit(feedItem{Kind: "presence", Data: entries})
}

func userEntries(users map[string]int) []UserEntry {
	entries := make([]UserEntry, 0, len(users))
	for nick, count := range users {
		entries = append(entries, UserEntry{Nick: nick, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Nick < entries[j].Nick
	})
	return entries
}

func room(roomid string) broadcast.Broadcaster {
	mutexRooms.Lock()
	defer mutexRooms.Unlock()
	b, ok := roomChannels[roomid]
	if !ok {
		b = broadcast.NewBroadcaster(10)
		roomChannels[roomid] = b
	}
	return b
}
