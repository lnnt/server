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

	validMessage := len(message) > 1 && len(message) < 200
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
	submitMessage(roomid, post)
	c.JSON(http.StatusOK, post)
}

func streamRoom(c *gin.Context) {
	roomid := c.Param("roomid")
	nick := presenceNick(c.Query("nick"))
	listener := openListener(roomid)

	addUser(roomid, nick)
	// 逆序执行:先注销自己的监听,再广播移除后的最新列表给其余人。
	defer removeUser(roomid, nick)
	defer closeListener(roomid, listener)

	c.Stream(func(w io.Writer) bool {
		select {
		case item := <-listener:
			if feed, ok := item.(feedItem); ok {
				c.SSEvent(feed.Kind, feed.Data)
			}
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
