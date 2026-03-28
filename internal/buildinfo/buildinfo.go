package buildinfo

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
}

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
