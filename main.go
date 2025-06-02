package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"raftImplementation/raft/log"
	"strconv"
	"syscall"
	"time"
)

func main() {
	handleRequests()
	time.Sleep(1000000 * time.Second)
}

type Configuration struct {
	ElectionTimeoutTime int             `json:"electionTimeoutTime"`
	NumberOfServers     int             `json:"numberOfServers"`
	ServerLogs          [][]log.Message `json:"serverLogs"`
}

func handleRequests() {
	http.Handle("/startSimulation", http.HandlerFunc(startSimulation))
	http.Handle("/stopSimulation", http.HandlerFunc(stopSimulation))
	http.Handle("/stopServer", http.HandlerFunc(stopServer))
	http.Handle("/resumeServer", http.HandlerFunc(resumeServer))
	fmt.Println(http.ListenAndServe(":8081", nil))
}

var processes []*os.Process

func startSimulation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers")
	if r.Method != http.MethodOptions {
		content, configuration := processConfigFile(r)

		if configuration.NumberOfServers != 0 {
			startServers(configuration, content)
		} else {
			fmt.Println("Configuration file does not contain numberOfServers property")
		}
	}
}

func processConfigFile(r *http.Request) ([]byte, Configuration) {
	var buf bytes.Buffer
	configFile, _, err := r.FormFile("file")
	var content []byte
	if err == nil {
		defer configFile.Close()
		io.Copy(&buf, configFile)
		content = []byte(buf.String())
	} else {
		defaultConfigFile := "defaultConfiguration.json"
		file, err := os.Open(defaultConfigFile)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		content, err = io.ReadAll(file)
	}

	var configuration Configuration
	err = json.Unmarshal(content, &configuration)
	return content, configuration
}

func startServers(configuration Configuration, content []byte) {
	for i := 0; i < configuration.NumberOfServers; i++ {
		file, err := os.Create("errorOutput" + strconv.Itoa(i) + ".txt")
		if err != nil {
			fmt.Println("Error creating file:", err)
		}
		defer file.Close()

		cmd := exec.Command("go", "run", "./runServer/runServer.go", strconv.Itoa(i))
		cmd.Stdin = bytes.NewReader(content)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
		cmd.Stdout = file
		cmd.Stderr = file
		cmd.Start()
		processes = append(processes, cmd.Process)

		fmt.Println("Started server node on localhost:6000" + strconv.Itoa(i) + ". Process pid of the server is " + strconv.Itoa(cmd.Process.Pid))
	}
}

func stopSimulation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers")
	fmt.Println("Stopping Simulation")
	for _, process := range processes {
		if err := syscall.Kill(-process.Pid, syscall.SIGKILL); err != nil {
			panic(err)
		}
	}
	processes = []*os.Process{}
}

type Data struct {
	ServerIndex int
}

func stopServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers")
	postData := &Data{}
	err := json.NewDecoder(r.Body).Decode(postData)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	serverIndex := postData.ServerIndex
	fmt.Println("Stopping server ", serverIndex)
	cmd := exec.Command("pkill", "-STOP", "-P", strconv.Itoa(processes[serverIndex].Pid))
	cmd.Start()
}

func resumeServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers")
	postData := &Data{}
	err := json.NewDecoder(r.Body).Decode(postData)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	serverIndex := postData.ServerIndex
	fmt.Println("Resuming server ", serverIndex)
	cmd := exec.Command("pkill", "-CONT", "-P", strconv.Itoa(processes[serverIndex].Pid))
	cmd.Start()
}
