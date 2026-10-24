package computeruse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestCuaRuntimeHelperProcess(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "cua-helper" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var r struct {
			ID uint64 `json:"id"`
		}
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			os.Exit(2)
		}
		if mode == "hang" {
			time.Sleep(time.Minute)
		}
		if mode == "wrong-id" {
			r.ID++
		}
		revision := cuaSourceRevision
		if mode == "wrong-source" {
			revision = "unexpected-source"
		}
		fmt.Printf("{\"protocol\":1,\"driver\":\"cua\",\"version\":\"fixture\",\"source_revision\":%q,\"id\":%d,\"result\":{\"pid\":%d}}\n", revision, r.ID, os.Getpid())
	}
	os.Exit(0)
}

func cuaFixture(mode string) *CuaRuntime {
	return &CuaRuntime{binary: os.Args[0], args: []string{"-test.run=^TestCuaRuntimeHelperProcess$", "--", "cua-helper", mode}}
}

func TestCuaRuntimePrivateProcessBelongsToOneTask(t *testing.T) {
	c := cuaFixture("echo")
	first, closeFirst := context.WithCancel(t.Context())
	defer closeFirst()
	one, err := c.Call(t.Context(), first, CuaRequest{Action: "status"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	two, err := c.Call(t.Context(), first, CuaRequest{Action: "status"}, nil)
	if err != nil || string(one.Result) != string(two.Result) {
		t.Fatal("same task did not reuse its private worker", err)
	}
	old := c.process
	closeFirst()
	select {
	case <-old.done:
	case <-time.After(3 * time.Second):
		t.Fatal("ended task left its process alive")
	}
	if _, err := c.Call(t.Context(), first, CuaRequest{Action: "status"}, nil); err == nil {
		t.Fatal("closed task restarted a worker")
	}
	second, closeSecond := context.WithCancel(t.Context())
	defer closeSecond()
	three, err := c.Call(t.Context(), second, CuaRequest{Action: "status"}, nil)
	if err != nil || string(three.Result) == string(one.Result) {
		t.Fatal("new task reused the old process/token cache", err)
	}
}

func TestCuaRuntimeUncertainTransportIsNotReplayed(t *testing.T) {
	for _, mode := range []string{"hang", "wrong-id", "wrong-source"} {
		t.Run(mode, func(t *testing.T) {
			c := cuaFixture(mode)
			lifetime, closeTask := context.WithCancel(t.Context())
			defer closeTask()
			ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			_, err := c.Call(ctx, lifetime, CuaRequest{Action: "status"}, nil)
			var uncertain *CuaTransportError
			if !errors.As(err, &uncertain) || c.process != nil {
				t.Fatalf("uncertain action remained executable: %v", err)
			}
			if _, err := c.Call(t.Context(), lifetime, CuaRequest{Action: "status"}, nil); err == nil || c.process != nil {
				t.Fatal("failed channel restarted automatically")
			}
		})
	}
}

func TestCuaParallelTasksKeepIndependentWorkersAndTokens(t *testing.T) {
	root := cuaFixture("echo")
	root.tasks = make(map[context.Context]*CuaRuntime)
	first, closeFirst := context.WithCancel(t.Context())
	defer closeFirst()
	second, closeSecond := context.WithCancel(t.Context())
	defer closeSecond()
	call := func(lifetime context.Context) CuaReply {
		t.Helper()
		reply, err := root.Call(t.Context(), lifetime, CuaRequest{Action: "status"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return reply
	}
	one, two := call(first), call(second)
	if string(one.Result) == string(two.Result) || string(call(first).Result) != string(one.Result) {
		t.Fatal("parallel tasks replaced another task's worker/token session")
	}
	root.mu.Lock()
	old := root.tasks[first].process
	root.mu.Unlock()
	closeFirst()
	select {
	case <-old.done:
	case <-time.After(3 * time.Second):
		t.Fatal("finished task's independent worker was not reaped")
	}
	if string(call(second).Result) != string(two.Result) {
		t.Fatal("finishing one task reset another task's worker")
	}
}
