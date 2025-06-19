package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
)

type Configuration struct {
	ElectionTimeoutTime int      `json:"electionTimeoutTime"`
	NumberOfServers     int      `json:"numberOfServers"`
	ServerAddresses     []string `json:"serverAddresses"`
	ServerIndex         int      `json:"serverIndex"`
}

func ProcessConfigFile(configFile string) ([]byte, Configuration) {
	file, err := os.Open(configFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	var configuration Configuration
	err = json.Unmarshal(content, &configuration)
	if err != nil {
		log.Fatal(err)
	}
	return content, configuration
}

func main() {
	configFile := "prodConfiguration.json"
	content, configuration := ProcessConfigFile(configFile)
	if (configuration.NumberOfServers % 2) == 0 {
		fmt.Println("You specified even number of servers in configuration file. For stable program execution " +
			"use odd number of servers.")
	}
	processes := make([]*exec.Cmd, configuration.NumberOfServers)
	serverNum := configuration.ServerIndex

	file, err := os.Create("errorOutput" + strconv.Itoa(serverNum) + ".txt")
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	cmd := exec.Command("go", "run", "./runServer/runServer.go", strconv.Itoa(serverNum))
	cmd.Stdin = bytes.NewReader(content)
	cmd.Stdout = file
	cmd.Stderr = file
	cmd.Start()

	processes[0] = cmd
	fmt.Println(strconv.Itoa(cmd.Process.Pid) + "  ---  " + "localhost:5000" + strconv.Itoa(serverNum))
	err = processes[0].Wait()
	if err != nil {
		panic(err)
	}
}
