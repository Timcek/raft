package server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
)

const ACTION_CHANGE_TIMEOUT = 1
const ACTION_SEND_VOTE_REQUEST = 2
const ACTION_SEND_VOTE_REPLY = 3
const ACTION_SEND_HEARTBEAT_MESSAGE = 4
const ACTION_SEND_HEARTBEAT_REPLY = 5
const ACTION_SEND_APPEND_ENTRY_MESSAGE = 6
const ACTION_SEND_APPEND_ENTRY_REPLY = 7
const ACTION_BECOME_LEADER = 8
const ACTION_GRANT_VOTE = 9
const ACTION_UPDATE_SERVER_TERM = 10

type Message struct {
	Action        int    `json:"action"`
	From          int    `json:"from"`
	To            int    `json:"to"`
	Granted       bool   `json:"granted"`
	Success       bool   `json:"success"`
	ServerIndex   int    `json:"serverIndex"`
	TimeoutLength int    `json:"timeoutLength"`
	Term          int    `json:"term"`
	Data          string `json:"data"`
}

var messages = make(chan Message)

func startServerWebSocket(serverIndex int) {
	fmt.Println("Listening on websocket...")
	http.HandleFunc("/serverEvents", wsHandler)
	//TODO spremeniti je potrebno port
	err := http.ListenAndServe(":6000"+fmt.Sprintf("%v", serverIndex), nil)
	if err != nil {
		panic("ListenAndServe: " + err.Error())
	}
}

// Takes massages from messages channels and sends them over web socket.
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

// Upgrader is used to upgrade HTTP connections to WebSocket connections. visualisation
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (server *Server) sendElectionTimeoutChange(time int) {
	message := Message{
		Action:      ACTION_CHANGE_TIMEOUT,
		ServerIndex: server.serverAddressIndex,
		// We multiply by jsVisualizationSpeed to convert go election timer into time for visualisation, and we multiply
		// by 1000 to transform it into correct form
		TimeoutLength: int(float64(time)*jsVisualizationSpeed) * 1000,
	}
	messages <- message
}

func (server *Server) sendVoteRequest(to int) {
	message := Message{
		Action: ACTION_SEND_VOTE_REQUEST,
		From:   server.serverAddressIndex,
		To:     to,
	}
	messages <- message
}

func (server *Server) sendVoteReply(to int, granted bool) {
	//TODO tudi tole še popravi
	message := Message{
		Action:  ACTION_SEND_VOTE_REPLY,
		From:    server.serverAddressIndex,
		To:      to,
		Granted: granted,
	}
	messages <- message
}

func (server *Server) sendHeartbeat(to int) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action: ACTION_SEND_HEARTBEAT_MESSAGE,
		From:   server.serverAddressIndex,
		To:     to,
	}
	messages <- message
}

func (server *Server) sendHeartbeatReply(to int, success bool) {
	//TODO tole je potrebno še pravilno implementirati tale successfull
	//in pa preglej če te stvari delujejo pravilno tudi v javascriptu
	message := Message{
		Action:  ACTION_SEND_HEARTBEAT_REPLY,
		From:    server.serverAddressIndex,
		To:      to,
		Success: success,
	}
	messages <- message
}

func (server *Server) sendAppendEntry(to int) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action: ACTION_SEND_APPEND_ENTRY_MESSAGE,
		From:   server.serverAddressIndex,
		To:     to,
	}
	messages <- message
}

func (server *Server) sendAppendEntryReply(to int, success bool) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:  ACTION_SEND_APPEND_ENTRY_REPLY,
		From:    server.serverAddressIndex,
		To:      to,
		Success: success,
	}
	messages <- message
}

func (server *Server) sendBecomeLeader() {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:      ACTION_BECOME_LEADER,
		ServerIndex: server.serverAddressIndex,
	}
	messages <- message
}

func (server *Server) grantVote(serverIndex int) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:      ACTION_GRANT_VOTE,
		ServerIndex: server.serverAddressIndex,
		// From in this case has to begin with 1
		From: serverIndex + 1,
	}
	messages <- message
}

func (server *Server) updateServerTerm() {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:      ACTION_UPDATE_SERVER_TERM,
		ServerIndex: server.serverAddressIndex,
		Term:        server.currentTerm,
	}
	messages <- message
}
