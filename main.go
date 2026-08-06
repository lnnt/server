package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/gin-contrib/sse"
	"github.com/gin-gonic/gin"
)

// Event keeps a list of clients those are currently attached
// and broadcasting events to those clients.
type Event struct {
	// Events are pushed to this channel by the main events-gathering routine
	Message chan string

	// New client connections
	NewClients chan chan string

	// Closed client connections
	ClosedClients chan chan string

	// Total client connections
	TotalClients map[chan string]bool
}

// ClientChan New event messages are broadcast to all registered client connection channels
type ClientChan chan string

func main() {
	router := gin.Default()

	// Initialize new streaming server
	stream := NewServer()

	router.GET("/stream", func(c *gin.Context) {
		w := c.Writer
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // 即使没用 Nginx，这也是好习惯

		// 关键：显式发送 200 状态码
		c.Status(http.StatusOK)

		// 关键：强制刷新 Header 到网络栈
		w.Flush()

		clientChan := make(ClientChan)
		stream.NewClients <- clientChan

		defer func() {
			stream.ClosedClients <- clientChan
		}()

		// 使用 Context 监听断开
		ctx := c.Request.Context()

		lastID := 0
		c.Stream(func(iw io.Writer) bool {
			select {
			case <-ctx.Done():
				return false
			case msg, ok := <-clientChan:
				if !ok {
					return false
				}
				lastID++
				c.Render(http.StatusOK, sse.Event{
					Id:    fmt.Sprintf("%d", lastID),
					Event: "message",
					Data:  msg,
				})
				return true
			}
		})
	})

	router.POST("/*k", func(c *gin.Context) {
		data, err := c.GetRawData()
		if err != nil {
			log.Printf("Error reading request body: %v", err)
			c.String(http.StatusInternalServerError, "Error reading request body: %v", err)
			return
		}

		stream.Message <- string(data)
	})

	// Parse Static files'
	router.StaticFile("/", "./public/index.html")

	log.Fatal(router.Run(":10000"))
}

// NewServer initialize event and Start processing requests
func NewServer() (event *Event) {
	event = &Event{
		Message:       make(chan string),
		NewClients:    make(chan chan string),
		ClosedClients: make(chan chan string),
		TotalClients:  make(map[chan string]bool),
	}

	go event.listen()

	return
}

// It Listens to all incoming requests from clients.
// Handles addition and removal of clients and broadcast messages to clients.
func (stream *Event) listen() {
	for {
		select {
		// Add new available client
		case client := <-stream.NewClients:
			stream.TotalClients[client] = true
			log.Printf("Client added. %d registered clients", len(stream.TotalClients))

		// Remove closed client
		case client := <-stream.ClosedClients:
			delete(stream.TotalClients, client)
			close(client)
			log.Printf("Removed client. %d registered clients", len(stream.TotalClients))

		// Broadcast message to a client
		case eventMsg := <-stream.Message:
			for clientMessageChan := range stream.TotalClients {
				select {
				case clientMessageChan <- eventMsg:
					// Message sent successfully
				default:
					// Failed to send, dropping message
				}
			}
		}
	}
}

func (stream *Event) serveHTTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Initialize client channel
		clientChan := make(ClientChan)

		// Send new connection to event server
		stream.NewClients <- clientChan

		go func() {
			<-c.Writer.CloseNotify()

			// Drain client channel so that it does not block. Server may keep sending messages to this channel
			for range clientChan {
			}
			// Send closed connection to event server
			stream.ClosedClients <- clientChan
		}()

		c.Set("clientChan", clientChan)

		c.Next()
	}
}
