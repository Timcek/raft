package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"raftImplementation/raft/server"
	"strconv"
)

type Configuration struct {
	ElectionTimeoutTime int `json:"electionTimeoutTime"`
	NumberOfServers     int `json:"numberOfServers"`
}

func ProcessConfigFile(configFile string) Configuration {
	file, err := os.Open(configFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	var configuration Configuration
	err = json.Unmarshal(fileBytes, &configuration)
	if err != nil {
		log.Fatal(err)
	}
	return configuration
}

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
