package server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"math"
	"net/http"
	sgrpc "raftImplementation/raft/server/src"
	"sync"
	"time"
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
const ACTION_APPEND_TO_LOG = 11
const ACTION_COMMIT_LOG = 12
const ACTION_UPDATE_NEXT_INDEX = 13

type Message struct {
	Action        int    `json:"action"`
	From          int    `json:"from"`
	To            int    `json:"to"`
	ServerState   string `json:"serverState"`
	Granted       bool   `json:"granted"`
	Success       bool   `json:"success"`
	ServerIndex   int    `json:"serverIndex"`
	TimeoutLength int    `json:"timeoutLength"`
	Term          int    `json:"term"`
	Data          string `json:"data"`
	CommitIndex   int    `json:"commitIndex"`
	NextIndex     int    `json:"nextIndex"`
	LeaderIndex   int    `json:"leaderIndex"`
}

func (server *Server) startServerWebSocket(serverIndex int) {

	fmt.Println("Listening on websocket...")
	http.HandleFunc("/serverEvents", server.wsHandler)
	err := http.ListenAndServe("0.0.0.0:6000"+fmt.Sprintf("%v", serverIndex), nil)
	if err != nil {
		panic("ListenAndServe: " + err.Error())
	}
}

// Upgrader is used to upgrade HTTP connections to WebSocket connections. visualisation
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Takes massages from messages channels and sends them over web socket.
func (server *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP connection to a WebSocket connection
	w.Header().Set("Access-Control-Allow-Origin", "*")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading:", err)
		return
	}
	defer conn.Close()

	var wg sync.WaitGroup

	// Wait for messages and send them to client
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			message := <-server.messages
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
	}()

	// Listen for client messages and handle them
	wg.Add(1)
	type msg struct {
		Method string `json:"method"`
		Value  int    `json:"value"`
	}
	go func() {
		defer wg.Done()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				fmt.Println("Error reading message:", err)
				break
			}
			var jsonMessage msg
			err = json.Unmarshal(message, &jsonMessage)
			if err != nil {
				fmt.Println("error:", err)
			}
			if jsonMessage.Method == "request" {
				go func() {
					conn, err := grpc.NewClient(server.serverAddresses[server.serverAddressIndex], grpc.WithInsecure())
					if err != nil {
						panic(err)
					}
					defer conn.Close()
					grpcClient := sgrpc.NewServerServiceClient(conn)
					//TODO check this timeout how long should it be
					contextServer, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()
					appendEntryMessage := sgrpc.ClientRequestMessage{
						Message: "salkf",
					}
					_, err = grpcClient.ClientRequest(contextServer, &appendEntryMessage)
					if err != nil {
						panic(err)
					}
				}()
			} else if jsonMessage.Method == "timeout" {
				server.electionTimeout()
			} else if jsonMessage.Method == "restart" {
				server.resetElectionTimer()
				server.becomeCandidate()
			} else if jsonMessage.Method == "speedChange" {
				jsVisualizationSpeed = float64(1) / float64(jsonMessage.Value)
				if server.serverState != LEADER {
					randomNum := float64(server.electionTime.Sub(time.Now())) / float64(time.Millisecond) / float64(electionTimeoutTime)
					//server.resetElectionTimer()
					fmt.Println("random num:", randomNum)
					fmt.Println("time this circle:", randomNum*float64(electionTimeoutTime))
					fmt.Println("election timeout time:", electionTimeoutTime)
					electionTimeoutTime = int(math.Pow(jsVisualizationSpeed, -1) * 200)
					server.electionTimer.Reset(time.Millisecond * time.Duration(randomNum*float64(electionTimeoutTime)))
					server.electionTime = time.Now().Add(time.Millisecond * time.Duration(randomNum*float64(electionTimeoutTime)))
					server.sendElectionTimeoutChange(int(randomNum * float64(electionTimeoutTime)))
				} else {
					timeProcentage := float64(server.heartbeatTime.Sub(time.Now())) / float64(time.Millisecond) / float64(electionTimeoutTime/4)
					electionTimeoutTime = int(math.Pow(jsVisualizationSpeed, -1) * 200)
					server.heartbeatTimer.Reset(time.Millisecond * time.Duration(timeProcentage*float64(electionTimeoutTime/4)))
					server.heartbeatTime = time.Now().Add(time.Millisecond * time.Duration(timeProcentage*float64(electionTimeoutTime/4)))
				}
				fmt.Println(jsonMessage.Value)
			} else if jsonMessage.Method == "stopServer" {
				server.stopElectionTimer()
			} else if jsonMessage.Method == "resumeServer" {
				if server.serverState != LEADER {
					server.resetElectionTimer()
				}
			}
		}
	}()

	wg.Wait()
}

func (server *Server) sendElectionTimeoutChange(time int) {
	message := Message{
		Action:      ACTION_CHANGE_TIMEOUT,
		ServerIndex: server.serverAddressIndex,
		// We multiply by jsVisualizationSpeed to convert go election timer into time for visualisation, and we multiply
		// by 1000 to transform it into correct form
		TimeoutLength: int(float64(time)*jsVisualizationSpeed) * 1000,
	}
	server.messages <- message
}

func (server *Server) sendVoteRequest(to int) {
	message := Message{
		Action: ACTION_SEND_VOTE_REQUEST,
		From:   server.serverAddressIndex,
		To:     to,
	}
	server.messages <- message
}

func (server *Server) sendVoteReply(to int, granted bool) {
	//TODO tudi tole še popravi
	message := Message{
		Action:  ACTION_SEND_VOTE_REPLY,
		From:    server.serverAddressIndex,
		To:      to,
		Granted: granted,
	}
	if server.serverState == FOLLOWER {
		message.ServerState = "follower"
	} else if server.serverState == CANDIDATE {
		// We send follower here so the visualisation looks nicer.
		message.ServerState = "follower"
	} else if server.serverState == LEADER {
		message.ServerState = "leader"
	}
	server.messages <- message
}

func (server *Server) sendHeartbeat(to int) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action: ACTION_SEND_HEARTBEAT_MESSAGE,
		From:   server.serverAddressIndex,
		To:     to,
	}
	server.messages <- message
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

	if server.serverState == FOLLOWER {
		message.ServerState = "follower"
	} else if server.serverState == CANDIDATE {
		message.ServerState = "candidate"
	} else if server.serverState == LEADER {
		message.ServerState = "leader"
	}

	server.messages <- message
}

func (server *Server) sendAppendEntry(to int) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action: ACTION_SEND_APPEND_ENTRY_MESSAGE,
		From:   server.serverAddressIndex,
		To:     to,
	}
	server.messages <- message
}

func (server *Server) sendAppendEntryReply(to int, success bool) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:  ACTION_SEND_APPEND_ENTRY_REPLY,
		From:    server.serverAddressIndex,
		To:      to,
		Success: success,
	}

	if server.serverState == FOLLOWER {
		message.ServerState = "follower"
	} else if server.serverState == CANDIDATE {
		message.ServerState = "candidate"
	} else if server.serverState == LEADER {
		message.ServerState = "leader"
	}

	server.messages <- message
}

func (server *Server) sendBecomeLeader() {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:      ACTION_BECOME_LEADER,
		ServerIndex: server.serverAddressIndex,
	}
	server.messages <- message
}

func (server *Server) grantVote(serverIndex int) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:      ACTION_GRANT_VOTE,
		ServerIndex: server.serverAddressIndex,
		// From in this case has to begin with 1
		From: serverIndex + 1,
	}
	server.messages <- message
}

func (server *Server) updateServerTerm() {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:      ACTION_UPDATE_SERVER_TERM,
		ServerIndex: server.serverAddressIndex,
		Term:        server.currentTerm,
	}
	server.messages <- message
}

func (server *Server) appendToServerLog(term int, data string) {
	//TODO tole je potrebno še pravilno implementirati
	message := Message{
		Action:      ACTION_APPEND_TO_LOG,
		ServerIndex: server.serverAddressIndex,
		Term:        term,
		Data:        data,
	}
	server.messages <- message
}

func (server *Server) commitLog() {
	message := Message{
		Action:      ACTION_COMMIT_LOG,
		ServerIndex: server.serverAddressIndex,
		CommitIndex: server.commitIndex + 1,
	}
	server.messages <- message
}

func (server *Server) sendNextIndex(index int) {
	message := Message{
		Action:      ACTION_UPDATE_NEXT_INDEX,
		NextIndex:   server.nextIndex[index],
		LeaderIndex: server.serverAddressIndex,
		To:          index,
	}
	server.messages <- message
}
