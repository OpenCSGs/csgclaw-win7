package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "child" {
		time.Sleep(30 * time.Minute)
		return
	}
	child := exec.Command(os.Args[0], "child")
	if e := child.Start(); e != nil {
		panic(e)
	}
	pids := []int{os.Getpid(), child.Process.Pid}
	b, _ := json.Marshal(pids)
	if e := os.WriteFile(os.Args[1], b, 0600); e != nil {
		panic(e)
	}
	fmt.Println("WIN7_WEBDRIVER_PROCESS_TREE_READY", pids)
	time.Sleep(30 * time.Minute)
}
