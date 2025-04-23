package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"math/rand"
	"net/http"
	"time"
)

const electionTimeoutTime = 1000

// Upgrader is used to upgrade HTTP connections to WebSocket connections.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Message struct {
	Action        int    `json:"action"`
	From          int    `json:"from"`
	To            int    `json:"to"`
	ServerIndex   int    `json:"serverIndex"`
	TimeoutLength int    `json:"timeoutLength"`
	Data          string `json:"data"`
}

var messages = make(chan Message)

func createMessages() {
	for {
		for i := 0; i < 5; i++ {
			message := Message{
				Action:      1,
				ServerIndex: i,
				Data:        "testtt",
				// We multiply by 100 to transform received time into correct time for the simulation (Raft uses milliseconds for timeouts but the simulation uses something between milliseconds and microseconds).
				TimeoutLength: (rand.Intn(electionTimeoutTime/2) + electionTimeoutTime/2) * 100,
			}
			messages <- message
		}
		time.Sleep(time.Duration(rand.Intn(2)+1) * time.Second)
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP connection to a WebSocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading:", err)
		return
	}
	defer conn.Close()
	// Listen for incoming messages
	for {
		message := <-messages
		jsonBytes, err := json.Marshal(message)
		if err != nil {
			panic(err)
		}
		// Echo the message back to the client
		if err := conn.WriteMessage(websocket.TextMessage, jsonBytes); err != nil {
			fmt.Println("Error writing message:", err)
			break
		}
	}
}

func websocketListen() {
	//when a client connects to the `/length` path, the `websocket.Handler` uses
	//the logic in function created earlier to handle WebSocket communication.
	http.HandleFunc("/ws", wsHandler)
	fmt.Println("Listening on websocket...")
	err := http.ListenAndServe(":6711", nil)
	if err != nil {
		panic("ListenAndServe: " + err.Error())
	}
}

func main() {
	go createMessages()
	go createMessages()
	go createMessages()
	go createMessages()
	websocketListen()
}
