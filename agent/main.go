package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ReplicaInfo struct {
	ID        string `json:"id"`
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
	CmdPath   string `json:"cmd_path"`
}

var (
	mu       sync.Mutex
	replicas = make(map[string]*exec.Cmd)
)

func listHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	res := []ReplicaInfo{}
	for id, cmd := range replicas {
		info := ReplicaInfo{
			ID:        id,
			PID:       cmd.Process.Pid,
			StartTime: time.Now().Format(time.RFC3339),
			CmdPath:   cmd.Path,
		}
		res = append(res, info)
	}

	json.NewEncoder(w).Encode(res)
}

func startHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	id := uuid.New().String()
	cmd := exec.Command("./bin/workload") // путь к workload
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	replicas[id] = cmd
	w.Write([]byte(id))
}

func stopHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	mu.Lock()
	defer mu.Unlock()

	cmd, ok := replicas[id]
	if !ok {
		http.Error(w, "replica not found", 404)
		return
	}

	cmd.Process.Kill()
	delete(replicas, id)
	w.Write([]byte("ok"))
}

func main() {
	port := "9000"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	http.HandleFunc("/list", listHandler)
	http.HandleFunc("/start", startHandler)
	http.HandleFunc("/stop", stopHandler)

	fmt.Println("Agent running on :" + port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("Failed to start agent:", err)
		os.Exit(1)
	}
}
