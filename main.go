package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

func main() {
	var processes [5]*exec.Cmd
	for i := 0; i < 5; i++ {
		file, err := os.Create("errorOutput" + strconv.Itoa(i) + ".txt")
		if err != nil {
			fmt.Println("Error creating file:", err)
			return
		}
		defer file.Close()

		cmd := exec.Command("go", "run", "runServer.go", strconv.Itoa(i))
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
