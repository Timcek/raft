package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func main() {
	configFile := "configuration.json"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}
	/*configuration := ProcessConfigFile(configFile)
	if (configuration.NumberOfServers % 2) == 0 {
		fmt.Println("You specified even number of servers in configuration file. For stable program execution " +
			"use odd number of servers.")
	}*/
	//processes := 5
	for i := 0; i < 5; i++ {
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

		//processes[i] = cmd
		fmt.Println(strconv.Itoa(cmd.Process.Pid) + "  ---  " + "localhost:5000" + strconv.Itoa(i))
	}
	time.Sleep(1000000 * time.Second)
	/*for _, process := range processes {
		err := process.Wait()
		if err != nil {
			panic(err)
		}
	}*/
}
