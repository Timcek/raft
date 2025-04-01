package main

import (
	"os"
	"raftImplementation/raft/server"
	"strconv"
)

// Function used to start the server. The function gets called from main.go by starting a completely new OS process
func main() {
	addresses := []string{"127.0.0.1:50000", "127.0.0.1:50001", "127.0.0.1:50002", "127.0.0.1:50003", "127.0.0.1:50004"}
	serverIndex, _ := strconv.Atoi(os.Args[1])
	server.CreateServer(serverIndex, addresses)
}
