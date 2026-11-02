package sandbox

import "os"

// hostPath is the PATH of every child process that runs on the host.
const hostPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

// hostEnvironment returns the environment for a host child process.
// It copies only HOME, USER, LOGNAME, LANG, TERM, and TMPDIR and sets PATH.
// It drops NONO_* overrides, Git configuration, and cloud credentials.
func hostEnvironment() map[string]string {
	env := make(map[string]string)
	for _, key := range []string{"HOME", "USER", "LOGNAME", "LANG", "TERM", "TMPDIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	env["PATH"] = hostPath
	return env
}
