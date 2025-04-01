package main

import (
	"fmt"
	"os"
	"raftImplementation/raft/server"
	"strconv"
)

// Function used to start the server. The function gets called from main.go by starting a completely new OS process
func main() {
	configuration := ProcessConfigFile(os.Args[2])
	addresses := make([]string, configuration.NumberOfServers)
	for i := 0; i < configuration.NumberOfServers; i++ {
		addresses[i] = fmt.Sprintf("127.0.0.1:5000%d", i)
	}
	serverIndex, _ := strconv.Atoi(os.Args[1])
	server.CreateServer(serverIndex, addresses)
}
