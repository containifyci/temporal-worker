### Steps to run this sample:
1) Run a [Temporal service](https://github.com/temporalio/samples-go/tree/main/#how-to-use).
2) Run the following command to start the worker
```
go run helloworld/worker/main.go
```
3) Run the following command to start the example
```
go run helloworld/starter/main.go
```

# Completed Features

* Unit test for RunEngineCI activity (runs 'engine-ci version')
* Auto-download engine-ci from GitHub releases if not available in PATH
* Refactored package structure with reusable activities:
  - `pkg/activities/git` - Generic git operations (CloneRepo)
  - `pkg/activities/filesystem` - Generic filesystem operations (CleanupDirectory)
  - `pkg/workflows/engineci` - Engine-CI specific logic (RunEngineCI) 
