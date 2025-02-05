package main

import (
	"context"
	"log"

	"github.com/containifyci/temporal-worker/pkg/workflows/github"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

func main() {
	// The client is a heavyweight object that should be created once per process.
	c, err := client.Dial(client.Options{})
	if err != nil {
		log.Fatalln("Unable to create client", err)
	}
	defer c.Close()

	workflowOptions := client.StartWorkflowOptions{
		ID:                       "queue_workflowID",
		TaskQueue:                "hello-world",
		WorkflowIDReusePolicy:    enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		WorkflowIDConflictPolicy: enums.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
		RetryPolicy:              nil,
	}

	_, err = c.SignalWithStartWorkflow(context.Background(), workflowOptions.ID, github.PullRequestReviewSignal, "pr 1", workflowOptions, github.PullRequestQueueWorkflow)

	// we, err := c.ExecuteWorkflow(context.Background(), workflowOptions, workflows.QueueWorkflow, "Temporal")
	if err != nil {
		log.Fatalln("Unable to execute workflow", err)
	}

	// log.Println("Started workflow", "WorkflowID", we.GetID(), "RunID", we.GetRunID())

	// // Synchronously wait for the workflow completion.
	// var result string
	// err = we.Get(context.Background(), &result)
	// if err != nil {
	// 	log.Fatalln("Unable get workflow result", err)
	// }
	// log.Println("Workflow result:", result)
	log.Println("Workflow started")
}
