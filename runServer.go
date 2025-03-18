package main

import (
	"os"
	"raft/server"
	"strconv"
)

// Function used to start the server. The function gets called from main.go by starting a completely new OS process
func main() {
	addresses := []string{"127.0.0.1:50000", "127.0.0.1:50001", "127.0.0.1:50002", "127.0.0.1:50003", "127.0.0.1:50004"}
	serverIndex := os.Args[1]
	index, _ := strconv.Atoi(os.Args[1])
	addresses = append(addresses[:index], addresses[index+1:]...)
	server.CreateServer("127.0.0.1:5000"+serverIndex, addresses)
}
