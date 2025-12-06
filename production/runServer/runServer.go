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
	ElectionTimeoutTime int      `json:"electionTimeoutTime"`
	NumberOfServers     int      `json:"numberOfServers"`
	ServerAddresses     []string `json:"serverAddresses"`
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
	addresses := configuration.ServerAddresses
	serverIndex, _ := strconv.Atoi(os.Args[1])
	server.CreateServer(serverIndex, addresses)
}
