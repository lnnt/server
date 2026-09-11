package main

import (
	"sort"
	"sync"
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

// chatRoom 维护一个房间内的所有 SSE 监听者与在线用户。
// 不再使用 go-broadcast:私聊需要按 nick 定向投递,
// 自己管理监听者可以在同一把锁里完成「查人 + 投递」。
type chatRoom struct {
	mu        sync.Mutex
	listeners map[chan feedItem]string // 监听者 -> nick(同名多标签页各占一条)
	users     map[string]int           // nick -> 连接数
}

var (
	mutexRooms sync.Mutex
	rooms      = make(map[string]*chatRoom)
)

func room(roomid string) *chatRoom {
	mutexRooms.Lock()
	defer mutexRooms.Unlock()
	r, ok := rooms[roomid]
	if !ok {
		r = &chatRoom{
			listeners: make(map[chan feedItem]string),
			users:     make(map[string]int),
		}
		rooms[roomid] = r
	}
	return r
}

// openListener 注册一个 SSE 监听者,nick 用于私聊定向投递。
func openListener(roomid, nick string) chan feedItem {
	r := room(roomid)
	listener := make(chan feedItem, listenerBuffer)

	r.mu.Lock()
	r.listeners[listener] = nick
	r.mu.Unlock()
	return listener
}

func closeListener(roomid string, listener chan feedItem) {
	r := room(roomid)

	// 投递都在 r.mu 内进行且从不阻塞(见 sendLocked),
	// 拿到锁时不可能有投递正在进行,close 不会撞上 send on closed channel。
	r.mu.Lock()
	delete(r.listeners, listener)
	r.mu.Unlock()
	close(listener)
}

// sendLocked 把 item 投给 nick 满足 keep 的所有监听者,须在持有 r.mu 时调用。
//
// 非阻塞发送:通道满(客户端太慢或已断开尚未注销)时丢弃该条,
// 宁可对单个慢客户端丢消息,也不阻塞整个房间或其他请求。
func (r *chatRoom) sendLocked(item feedItem, keep func(nick string) bool) {
	for listener, nick := range r.listeners {
		if !keep(nick) {
			continue
		}
		select {
		case listener <- item:
		default:
		}
	}
}

// broadcast 向房间内全部监听者投递。
func (r *chatRoom) broadcast(item feedItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sendLocked(item, func(string) bool { return true })
}

func submitMessage(roomid string, post any) {
	room(roomid).broadcast(feedItem{Kind: "message", Data: post})
}

// onlineNicks 返回房间内当前持有 SSE 连接的昵称(去重,无序)。
// 以监听者而不是在线表为准:能投递到才算在线。
func onlineNicks(roomid string) []string {
	r := room(roomid)
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[string]bool, len(r.listeners))
	nicks := make([]string, 0, len(r.listeners))
	for _, nick := range r.listeners {
		if !seen[nick] {
			seen[nick] = true
			nicks = append(nicks, nick)
		}
	}
	return nicks
}

// submitPrivateMessage 只把消息投给 to 和 from 本人的连接。
// 返回 to 是否有在线连接;不在线时什么都没投递。
func submitPrivateMessage(roomid, from, to string, post any) bool {
	r := room(roomid)
	item := feedItem{Kind: "message", Data: post}

	// 在线判断与投递在同一把锁内完成,
	// 避免「刚查到在线、对方随即断开」时把消息寄给空气。
	r.mu.Lock()
	defer r.mu.Unlock()

	online := false
	for _, nick := range r.listeners {
		if nick == to {
			online = true
			break
		}
	}
	if !online {
		return false
	}

	// 发送者自己也收一份(与原广播行为一致),便于确认已送达。
	r.sendLocked(item, func(nick string) bool { return nick == to || nick == from })
	return true
}

// addUser 记录一个在线连接,并向房间内所有监听者广播最新的用户列表。
func addUser(roomid, nick string) {
	r := room(roomid)

	r.mu.Lock()
	r.users[nick]++
	entries := userEntries(r.users)
	r.mu.Unlock()

	r.broadcast(feedItem{Kind: "presence", Data: entries})
}

// removeUser 在一个连接断开时调用,计数归零则从列表中移除。
func removeUser(roomid, nick string) {
	r := room(roomid)

	r.mu.Lock()
	if r.users[nick]--; r.users[nick] <= 0 {
		delete(r.users, nick)
	}
	entries := userEntries(r.users)
	r.mu.Unlock()

	r.broadcast(feedItem{Kind: "presence", Data: entries})
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
