// SPDX-FileCopyrightText: 2023 Isaak Tsalicoglou <isaak@waseigo.com>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var mu sync.Mutex
var wg sync.WaitGroup
var firstRun bool
var isAlreadyRunning bool

func expandTilde(path string) (string, error) {
	if path[:2] == "~/" {
		usr, err := user.Current()
		if err != nil {
			return "", err
		}
		path = filepath.Join(usr.HomeDir, path[2:])
	}
	return path, nil
}

func genTimestamp() string {
	now := time.Now()
	return now.Format(time.RFC3339) + "\t"
}

func getGitRepoPath() (string, error) {
	gitRepoPath := os.Getenv("GIT_REPO_PATH")
	return expandTilde(gitRepoPath)
}

func authenticate(w http.ResponseWriter, r *http.Request) error {
	apiKeyHeader := r.Header.Get("X-Gitlab-Token")
	fmt.Println(genTimestamp() + "🤝 Received a valid secret token from Gitlab")

	apiKey := os.Getenv("WEBHOOK_SECRET_TOKEN")
	if apiKeyHeader != apiKey {
		http.Error(w, genTimestamp()+"💩 Invalid secret token!", http.StatusForbidden)
		return fmt.Errorf(genTimestamp() + "🚫 Invalid secret token")
	}
	return nil
}

func gitPull(gitRepoPath string) (bool, error) {
	fmt.Println(genTimestamp() + "📡 Performing 'git pull'…")

	cmdGitPull := exec.Command("git", "pull")
	cmdGitPull.Dir = gitRepoPath
	stdout, err := cmdGitPull.StdoutPipe()
	if err != nil {
		return false, fmt.Errorf(genTimestamp()+"💩 Error creating StdoutPipe for 'git pull': %v", err)
	}

	err = cmdGitPull.Start()
	if err != nil {
		return false, fmt.Errorf(genTimestamp()+"💩 Error starting 'git pull': %v", err)
	}

	outputBytes, err := io.ReadAll(stdout)
	if err != nil {
		return false, fmt.Errorf(genTimestamp()+"💩 Error reading 'git pull' output: %v", err)
	}

	err = cmdGitPull.Wait()
	if err != nil {
		return false, fmt.Errorf(genTimestamp()+"💩 Error waiting for 'git pull': %v", err)
	}

	output := string(outputBytes)
	if strings.Contains(output, "Already up to date.") {
		fmt.Println(genTimestamp() + "🤷 The repository is already up to date. No further actions needed.")
		return false, nil
	}

	return true, nil
}

func npmStart(gitRepoPath string) error {
	cmd := exec.Command("npm", "start")
	cmd.Dir = gitRepoPath

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := cmd.Run()
		if err != nil {
			fmt.Println(genTimestamp()+"💩 Error running 'npm start':", err)
		} else {
			fmt.Println(genTimestamp() + "🚀 'npm start' is running!")
		}
	}()

	return nil
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {

	err := authenticate(w, r)
	if err != nil {
		return
	}

	fmt.Fprintln(w, genTimestamp()+" Webhook request received")

	go func() {
		fmt.Println(genTimestamp() + "⚠️ 'git push' detected. Performing 'git pull' to see if an update is required.")

		err := updatePipeline()
		if err != nil {
			fmt.Println(genTimestamp()+"💩 Error during update pipeline:", err)
			return
		}
	}()
}

func killNpmStartIfRunning() error {

	port := getNpmPort()
	fmt.Println(genTimestamp() + "💣 Killing 'npm start'…")
	killProcessOnPort(port)
	isAlreadyRunning = false

	return nil
}

func getProcessIDOnPort(port string) (int, error) {

	// Extract PID of the running NextJS app from the output of netstat
	cmd := exec.Command("sh", "-c", fmt.Sprintf("netstat -tlpn | grep ':%s' | sed -E 's/^.* ([^\\/]+)\\/.*/\\1/'", port))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf(genTimestamp()+"💩 Error creating StdoutPipe: %v\n", err)
		return 0, err
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf(genTimestamp()+"💩 Error starting command: %v\n", err)
		return 0, err
	}

	output, err := io.ReadAll(stdout)
	if err != nil {
		fmt.Printf(genTimestamp()+"💩 Error reading output: %v\n", err)
		return 0, err
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf(genTimestamp()+"💩 Error waiting for command: %v\n", err)
		return 0, err
	}

	if strings.TrimSpace(string(output)) == "" {
		fmt.Printf(genTimestamp()+"🤷 No process found on port %s\n", port)
		return 0, nil
	}

	var pid int
	pid, err = strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		fmt.Printf(genTimestamp()+"💩 Error parsing PID: %v\n", err)
		return 0, err
	}

	fmt.Println(genTimestamp() + "✅ Found PID of process running on port " + port + ": " + strconv.Itoa(pid))
	return pid, nil
}

func killProcessOnPort(port string) error {
	pid, err := getProcessIDOnPort(port)
	if err != nil {
		return err
	}

	if pid != 0 {
		fmt.Printf(genTimestamp()+"💣 Killing process with PID %d on port %s\n", pid, port)
		return exec.Command("kill", fmt.Sprintf("%d", pid)).Run()
	}

	fmt.Printf(genTimestamp()+"🤷 No process found on port %s\n", port)
	return nil
}

func runNpmInstall(gitRepoPath string) error {
	fmt.Println(genTimestamp() + "🛠️  Running 'npm install'…")
	cmd := exec.Command("npm", "install")
	cmd.Dir = gitRepoPath

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(genTimestamp()+"💩 Error running 'npm install': %v", err)
	} else {
		fmt.Println(genTimestamp() + "✅ 'npm install' completed")
	}

	return nil
}

func runNpmBuild(gitRepoPath string) error {
	fmt.Println(genTimestamp() + "🏗️  Running 'npm run build'…")
	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = gitRepoPath

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(genTimestamp()+"💩 Error running 'npm run build': %v", err)
	} else {
		fmt.Println(genTimestamp() + "✅ 'npm run build' completed")
	}

	return nil
}

func updatePipeline() error {
	mu.Lock()
	defer mu.Unlock()

	gitRepoPath, err := getGitRepoPath()
	if err != nil {
		return fmt.Errorf(genTimestamp()+"💩 Error getting Git repository path: %v", err)
	}

	thereAreGitChanges, err := gitPull(gitRepoPath)
	if err != nil {
		return fmt.Errorf(genTimestamp()+"💩 Error during 'git pull': %v", err)
	}
	fmt.Println(genTimestamp() + "✅ Git pull completed")

	buildRequired := thereAreGitChanges || (firstRun && !isAlreadyRunning)

	if buildRequired {
		fmt.Println(genTimestamp() + "🔃 Rebuilding is required!")

		err = killNpmStartIfRunning()
		if err != nil {
			return fmt.Errorf(genTimestamp()+"💩 Error killing already-running 'npm start': %v", err)
		}

		err = runNpmInstall(gitRepoPath)
		if err != nil {
			return fmt.Errorf(genTimestamp()+"💩 Error running 'npm install': %v", err)
		}

		err = runNpmBuild(gitRepoPath)
		if err != nil {
			return fmt.Errorf(genTimestamp()+"💩 Error running 'npm run build': %v", err)
		}

		firstRun = false
	} else {
		fmt.Println(genTimestamp() + "😴 No changes in the Git repository since the last 'npm run build'. Skipping update.")
		return nil
	}

	err = npmStart(gitRepoPath)
	if err != nil {
		return fmt.Errorf(genTimestamp()+"💩 Error in 'npm start': %v", err)
	}

	fmt.Println(genTimestamp() + "🥳 Update completed and 'npm start' issued.")
	return nil
}

func getNpmPort() string {
	port := os.Getenv("NEXTJS_PORT")
	if port == "" {
		port = "3000"
	}
	return port
}

func getWebhookPort() string {
	port := os.Getenv("WEBHOOK_PORT")
	if port == "" {
		port = "8000"
	}
	return port
}

func main() {
	firstRun = true

	fmt.Println(genTimestamp() + "🛋️ Performing initial setup…")

	npmPort := getNpmPort()
	fmt.Println(genTimestamp()+"🔎 Checking whether 'npm start' is already running on port", npmPort)
	pid, _ := getProcessIDOnPort(npmPort)
	isAlreadyRunning = (pid != 0)

	if !isAlreadyRunning {
		fmt.Println(genTimestamp() + "🤔 The app was not running already; time to update and start it!")
		err := updatePipeline()
		if err != nil {
			fmt.Println(genTimestamp()+"💩 Error during initial setup:", err)
			return
		}
	} else {
		fmt.Println(genTimestamp() + "🤷 The app was running already; will not update!")
	}

	webhookPort := getWebhookPort()
	fmt.Println(genTimestamp()+"🚀 Starting the webhook server on port", webhookPort)

	http.HandleFunc("/webhook", webhookHandler)

	err := http.ListenAndServe(":"+webhookPort, nil)
	if err != nil {
		fmt.Println(genTimestamp()+"💩 Error starting the webhook server:", err)
	} else {
		fmt.Println(genTimestamp() + "📟 Webhook server started!")
	}

}
