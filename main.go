package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 연결된 클라이언트를 관리하는 구조체
type Message struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Time    string `json:"time"`
}
type ClientManager struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.Mutex
}

var manager = ClientManager{
	clients:    make(map[*websocket.Conn]bool),
	broadcast:  make(chan []byte),
	register:   make(chan *websocket.Conn),
	unregister: make(chan *websocket.Conn),
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 클라이언트 관리 루프
func (manager *ClientManager) start() {
	for {
		select {
		case conn := <-manager.register:
			manager.mu.Lock()
			manager.clients[conn] = true
			manager.mu.Unlock()
			log.Println("클라이언트 연결됨:", conn.RemoteAddr())

		case conn := <-manager.unregister:
			manager.mu.Lock()
			if _, ok := manager.clients[conn]; ok {
				delete(manager.clients, conn)
				conn.Close()
				log.Println("클라이언트 연결 해제됨:", conn.RemoteAddr())
			}
			manager.mu.Unlock()

		case message := <-manager.broadcast:
			manager.mu.Lock()
			var msg Message
			if err := json.Unmarshal(message, &msg); err != nil {
				fmt.Println(err)
			}
			msg.Time = time.Now().Format("2006-01-02 15:04:05")

			jsonData, err := json.Marshal(msg)
			if err != nil {
				log.Println("JSON 인코딩 실패:", err)
				continue
			}

			for conn := range manager.clients {
				if err := conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
					log.Println("메시지 전송 오류:", err)
					conn.Close()
					delete(manager.clients, conn)
				}
			}
			manager.mu.Unlock()
		}
	}
}

// WebSocket 요청 처리
func serveWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket 업그레이드 실패:", err)
		return
	}
	manager.register <- conn

	defer func() {
		manager.unregister <- conn
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("메시지 읽기 실패:", err)
			return
		}
		manager.broadcast <- message // 메시지를 브로드캐스트 채널로 전달
	}
}

func main() {
	go manager.start()

	http.HandleFunc("/ws", serveWebSocket)
	log.Println("WebSocket 서버 실행 중... 포트: 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
