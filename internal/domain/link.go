package domain

type Link struct {
	LocalPath   string   `json:"local_path"`
	RemoteHost  string   `json:"remote_host"`
	RemotePath  string   `json:"remote_path"`
	Patterns    []string `json:"patterns"`
	ListenerPid int      `json:"listener_pid,omitempty"`
}

type Links map[string]Link
