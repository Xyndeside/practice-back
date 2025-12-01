package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type ReplicaInfo struct {
	ID        string `json:"id"`
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
	CmdPath   string `json:"cmd_path"`
	Agent     string `json:"agent"`
}

type Controller struct {
	agents      []string
	lastKnown   map[string]ReplicaInfo
	targetCount int
	mu          sync.Mutex
	httpClient  *http.Client
}

func NewController(agents []string, target int) *Controller {
	return &Controller{
		agents:      agents,
		targetCount: target,
		lastKnown:   make(map[string]ReplicaInfo),
		httpClient:  &http.Client{Timeout: 2 * time.Second},
	}
}

func (c *Controller) pollOnce() {
	found := make(map[string]ReplicaInfo)
	for _, agent := range c.agents {
		url := fmt.Sprintf("http://%s/list", agent)
		resp, err := c.httpClient.Get(url)
		if err != nil {
			// agent down — skip
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		type repFromAgent struct {
			ID        string `json:"id"`
			PID       int    `json:"pid"`
			StartTime string `json:"start_time"`
			CmdPath   string `json:"cmd_path"`
		}

		var raw []repFromAgent
		_ = json.Unmarshal(body, &raw)
		for _, r := range raw {
			info := ReplicaInfo{
				ID:        r.ID,
				PID:       r.PID,
				StartTime: r.StartTime,
				CmdPath:   r.CmdPath,
				Agent:     agent,
			}
			found[r.ID] = info
		}
	}

	c.mu.Lock()
	c.lastKnown = found
	c.mu.Unlock()
}

func (c *Controller) agentLoad() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	loads := make(map[string]int)
	for _, a := range c.agents {
		loads[a] = 0
	}
	for _, r := range c.lastKnown {
		loads[r.Agent]++
	}
	return loads
}

func (c *Controller) pickAgent() string {
	loads := c.agentLoad()
	minAgent := ""
	minVal := 1<<31 - 1
	for _, a := range c.agents {
		if loads[a] < minVal {
			minVal = loads[a]
			minAgent = a
		}
	}
	return minAgent
}

func (c *Controller) startReplica(n int) {
	for i := 0; i < n; i++ {
		agent := c.pickAgent()
		http.Post("http://"+agent+"/start", "application/json", nil)
	}
}

func (c *Controller) stopReplica(n int) {
	c.mu.Lock()
	reps := make([]ReplicaInfo, 0, len(c.lastKnown))
	for _, r := range c.lastKnown {
		reps = append(reps, r)
	}
	c.mu.Unlock()

	for i := 0; i < n && i < len(reps); i++ {
		r := reps[i]
		http.Post("http://"+r.Agent+"/stop?id="+r.ID, "", nil)
	}
}

func (c *Controller) reconcile() {
	c.mu.Lock()
	current := len(c.lastKnown)
	target := c.targetCount
	c.mu.Unlock()

	if current < target {
		c.startReplica(target - current)
	} else if current > target {
		c.stopReplica(current - target)
	}
}

func (c *Controller) statusHandler(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	json.NewEncoder(w).Encode(c.lastKnown)
}

func (c *Controller) scaleHandler(w http.ResponseWriter, r *http.Request) {
	nStr := r.URL.Query().Get("count")
	n, _ := strconv.Atoi(nStr)
	c.mu.Lock()
	c.targetCount = n
	c.mu.Unlock()
	w.Write([]byte("OK"))
}

func main() {
	agents := []string{"localhost:9000", "localhost:9001", "localhost:9002"} // добавь здесь все агенты
	controller := NewController(agents, 3)

	go func() {
		for {
			controller.pollOnce()
			controller.reconcile()
			time.Sleep(2 * time.Second)
		}
	}()

	http.HandleFunc("/status", controller.statusHandler)
	http.HandleFunc("/scale", controller.scaleHandler)

	fmt.Println("Controller running on :8000")
	http.ListenAndServe(":8000", nil)
}
