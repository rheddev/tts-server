package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Add CORS check
	CheckOrigin: func(r *http.Request) bool {
		return true // In production, replace with proper origin check
	},
}

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan Message
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mutex      sync.Mutex
}

var hub = Hub{
	clients:    make(map[*websocket.Conn]bool),
	broadcast:  make(chan Message),
	register:   make(chan *websocket.Conn),
	unregister: make(chan *websocket.Conn),
	mutex:      sync.Mutex{},
}

func (hub *Hub) run() {
	for {
		select {
		case client := <-hub.register:
			hub.mutex.Lock()
			hub.clients[client] = true
			hub.mutex.Unlock()
			log.Printf("Client connected. Total clients: %d", len(hub.clients))
		case client := <-hub.unregister:
			hub.mutex.Lock()
			if _, ok := hub.clients[client]; ok {
				delete(hub.clients, client)
				client.Close()
				log.Printf("Client disconnected. Total clients: %d", len(hub.clients))
			}
			hub.mutex.Unlock()
		case message := <-hub.broadcast:
			hub.mutex.Lock()
			messageJSON, err := json.Marshal(message)
			if err != nil {
				log.Printf("Error marshaling message: %v", err)
				hub.mutex.Unlock()
				continue
			}

			for client := range hub.clients {
				// Set write deadline
				client.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := client.WriteMessage(websocket.TextMessage, messageJSON); err != nil {
					log.Printf("Error writing message to client: %v", err)
					client.Close()
					delete(hub.clients, client)
				}
				addMessage(message.Name, message.Amount, message.Message)
			}
			hub.mutex.Unlock()
		}
	}
}

func listenHandler(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Error upgrading connection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return
	}

	hub.register <- ws

	defer func() {
		hub.unregister <- ws
		ws.Close()
	}()

	// Set read deadline
	ws.SetReadDeadline(time.Now().Add(24 * time.Hour))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(24 * time.Hour))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				log.Printf("Error sending ping: %v", err)
				return
			}
		default:
			if _, _, err := ws.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Error reading message: %v", err)
				}
				return
			}
		}
	}
}

func sendHandler(c *gin.Context) {
	var req Message
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Error binding JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Validate message
	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message cannot be empty"})
		return
	}

	hub.broadcast <- req
	c.JSON(http.StatusOK, gin.H{"status": "Message successfully sent"})
}
