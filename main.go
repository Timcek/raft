package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
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

func main() {
	configFile := "configuration.json"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}
	configuration := ProcessConfigFile(configFile)
	if (configuration.NumberOfServers % 2) == 0 {
		fmt.Println("You specified even number of servers in configuration file. For stable program execution " +
			"use odd number of servers.")
	}
	processes := make([]*exec.Cmd, configuration.NumberOfServers)
	for i := 0; i < configuration.NumberOfServers; i++ {
		file, err := os.Create("errorOutput" + strconv.Itoa(i) + ".txt")
		if err != nil {
			fmt.Println("Error creating file:", err)
			return
		}
		defer file.Close()

		cmd := exec.Command("go", "run", "runServer.go", strconv.Itoa(i), configFile)
		cmd.Stdout = file
		cmd.Stderr = file
		cmd.Start()

		processes[i] = cmd
		fmt.Println(strconv.Itoa(cmd.Process.Pid) + "  ---  " + "localhost:5000" + strconv.Itoa(i))
	}
	for _, process := range processes {
		err := process.Wait()
		if err != nil {
			panic(err)
		}
	}
}
