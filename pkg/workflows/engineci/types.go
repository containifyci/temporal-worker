package engineci

// EngineCIWorkflowInput contains all the information needed to run an Engine-CI job
type EngineCIWorkflowInput struct {
	GitRepoURL string
	GitRef     string
	RepoName   string
	EngineArgs []string
	Env        map[string]string
}

// EngineCIDetails contains the results of an Engine-CI execution
type EngineCIDetails struct {
	ExitCode    int
	Last50Lines string
}
