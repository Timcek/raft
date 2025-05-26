package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"raftImplementation/raft/server"
	"strconv"
)

type Config struct {
	ElectionTimeoutTime int `json:"electionTimeoutTime"`
	NumberOfServers     int `json:"numberOfServers"`
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
	server.CreateServer(serverIndex, addresses)
}
