package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"raftImplementation/raft/log"
	"raftImplementation/raft/server"
	"strconv"
)

type Config struct {
	NumberOfServers int             `json:"numberOfServers"`
	NextTerm        int             `json:"nextTerm"`
	ServerLogs      [][]log.Message `json:"serverLogs"`
}

// Function used to start the server. The function gets called from main.go by starting a completely new OS process
func main() {
	jsonData, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Println("Error reading stdin:", err)
		return
	}
	var configuration Config
	err = json.Unmarshal(jsonData, &configuration)
	if err != nil {
		panic(err)
	}
	addresses := make([]string, configuration.NumberOfServers)
	for i := 0; i < configuration.NumberOfServers; i++ {
		addresses[i] = fmt.Sprintf("127.0.0.1:5000%d", i)
	}
	serverIndex, _ := strconv.Atoi(os.Args[1])
	if len(configuration.ServerLogs) > serverIndex {
		server.CreateServer(serverIndex, addresses, configuration.ServerLogs[serverIndex], configuration.NextTerm-1)
	} else {
		server.CreateServer(serverIndex, addresses, []log.Message{}, configuration.NextTerm-1)
	}
}
