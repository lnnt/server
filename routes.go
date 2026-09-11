package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	mutexIP sync.Mutex
	ipHits  = map[string]int{}
)

func rateLimit(c *gin.Context) {
	ip := c.ClientIP()
	mutexIP.Lock()
	ipHits[ip]++
	value := ipHits[ip]
	mutexIP.Unlock()
	if value%50 == 0 {
		fmt.Printf("ip: %s, count: %d\n", ip, value)
	}
	if value >= 200 {
		if value%200 == 0 {
			fmt.Println("ip blocked")
		}
		c.Abort()
		c.String(http.StatusServiceUnavailable, "you were automatically banned :)")
	}
}

func index(c *gin.Context) {
	c.Redirect(http.StatusMovedPermanently, "/room/hn")
}

func roomGET(c *gin.Context) {
	roomid := c.Param("roomid")
	nick := c.Query("nick")
	if len(nick) < 2 {
		nick = ""
	}
	if len(nick) > 13 {
		nick = nick[0:12] + "..."
	}
	c.HTML(http.StatusOK, "room_login.templ.html", gin.H{
		"roomid": roomid,
		"nick":   nick,
	})

}

func roomPOST(c *gin.Context) {
	roomid := c.Param("roomid")
	nick := c.Query("nick")
	message := c.PostForm("message")
	message = strings.TrimSpace(message)

	validMessage := len(message) > 0 && len(message) < 200
	validNick := len(nick) > 1 && len(nick) < 14
	if !validMessage || !validNick {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "failed",
			"error":  "the message or nickname is too long",
		})
		return
	}

	post := gin.H{
		"nick":    html.EscapeString(nick),
		"message": html.EscapeString(message),
	}

	// "@user1 正文" → 定向私聊:只推给 user1 和发送者本人,其他人收不到。
	to, isDM, unknown := parseMention(message, onlineNicks(roomid))
	if isDM {
		from := presenceNick(nick)
		post["to"] = html.EscapeString(to)
		post["private"] = true
		if !submitPrivateMessage(roomid, from, to, post) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "failed",
				"error":  "@" + to + " is not online",
			})
			return
		}
		c.JSON(http.StatusOK, post)
		return
	}
	if unknown != "" {
		// 看着像私聊但对方不在线:宁可直接报错,也绝不把内容广播出去。
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "failed",
			"error":  "@" + unknown + " is not online",
		})
		return
	}

	submitMessage(roomid, post)
	c.JSON(http.StatusOK, post)
}

// parseMention 解析以 "@" 开头的消息,在线昵称列表由 onlineNicks 提供。
//
//	"@bob 正文"   → to="bob", isDM=true(最长匹配,支持带空格/省略号的昵称)
//	"@ghost 正文" → isDM=false, unknown="ghost"(形似私聊但对方不在线)
//	其余           → 普通消息("@bob" 这种没有正文的也算普通消息)
func parseMention(message string, online []string) (to string, isDM bool, unknown string) {
	if !strings.HasPrefix(message, "@") {
		return "", false, ""
	}

	// 优先最长匹配:"@bob cat 你好" 里 bob cat 和 bob 同时在线时给 bob cat。
	best := ""
	for _, nick := range online {
		prefix := "@" + nick + " "
		if len(message) > len(prefix) && strings.HasPrefix(message, prefix) && len(nick) > len(best) {
			best = nick
		}
	}
	if best != "" {
		return best, true, ""
	}

	space := strings.IndexByte(message, ' ')
	// 昵称至少 2 个字符(roomPOST 的约束),即 "@" + 昵称 + " " 的下标至少是 3;
	// "@ xxx"(空昵称)、"@x hi"(单字符)、"@bob"(没有正文)都按普通消息处理。
	if space < 3 {
		return "", false, ""
	}
	token := message[1:space]
	// 对方昵称过长时在线列表里是 "xxx..." 的截断形式,按显示形式再匹配一次。
	for _, nick := range online {
		if nick == presenceNick(token) {
			return nick, true, ""
		}
	}
	return "", false, token
}

func streamRoom(c *gin.Context) {
	roomid := c.Param("roomid")
	nick := presenceNick(c.Query("nick"))
	listener := openListener(roomid, nick)

	addUser(roomid, nick)
	// 逆序执行:先注销自己的监听,再广播移除后的最新列表给其余人。
	defer removeUser(roomid, nick)
	defer closeListener(roomid, listener)

	c.Stream(func(w io.Writer) bool {
		select {
		case item := <-listener:
			c.SSEvent(item.Kind, item.Data)
		case <-c.Request.Context().Done():
			return false
		}
		return true
	})
}

// presenceNick 与 roomGET 对 nick 的处理保持一致,
// 未填 nick 的连接(还没加入聊天的访客)统一显示为 (anonymous)。
func presenceNick(nick string) string {
	if len(nick) < 2 {
		return "(anonymous)"
	}
	if len(nick) > 13 {
		return nick[0:12] + "..."
	}
	return nick
}
