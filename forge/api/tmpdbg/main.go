package main

import (
	"context"
	"fmt"
	"os"

	"gamepanel/forge/internal/daemon"
)

func main() {
	token := os.Args[1]
	c, err := daemon.NewClient("http://127.0.0.1:9090", token)
	if err != nil {
		panic(err)
	}
	resp, err := c.CreateServer(context.Background(), "http://127.0.0.1:9090", token, daemon.CreateRequest{
		ServerID: "e166b0b9-1e06-4ea6-ad02-2e9a2c22aa50",
		Image:    "alpine:latest",
		Command:  []string{"sh", "-c", "sleep 300"},
		Ports:    []daemon.Port{{HostIP: "127.0.0.1", HostPort: 25732, ContainerPort: 25732, Protocol: "tcp"}},
		MemoryMB: 256, CPUShares: 1024, DiskMB: 512,
		UID: 1000, GID: 1000, Provider: "docker",
	})
	if err != nil {
		if re, ok := err.(*daemon.ResponseError); ok {
			fmt.Printf("STATUS=%d DETAILS=%s\n", re.StatusCode, re.Details)
			return
		}
		fmt.Printf("ERR=%v\n", err)
		return
	}
	fmt.Printf("OK %+v\n", resp)
}
