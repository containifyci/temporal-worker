package main

import (
	"fmt"
	"log"
	"os"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/containifyci/go-self-update/pkg/updater"
	"github.com/containifyci/temporal-worker/pkg/helloworld"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	fmt.Printf("temporal-worker %s, commit %s, built at %s\n", version, commit, date)
	// Check for command-line arguments
	command := "start"
	if len(os.Args) >= 2 {
		command = os.Args[1]
	}

	// Get the command
	switch command {
	case "update":
		u := updater.NewUpdater("temporal-worker", "containifyci", "temporal-worker", version)
		updated, err := u.SelfUpdate()
		if err != nil {
			log.Fatalln("Update failed:", err)
		}
		if updated {
			fmt.Println("Update completed successfully!")
			return
		}
		fmt.Println("Already up-to-date")
	default:
		start()
	}
}

func start() {
	// The client and worker are heavyweight objects that should be created once per process.
	c, err := client.Dial(client.Options{})
	if err != nil {
		log.Fatalln("Unable to create client", err)
	}
	defer c.Close()

	w := worker.New(c, "hello-world", worker.Options{})

	w.RegisterWorkflow(helloworld.Workflow)
	w.RegisterActivity(helloworld.Activity)

	err = w.Run(worker.InterruptCh())
	if err != nil {
		log.Fatalln("Unable to start worker", err)
	}
}
