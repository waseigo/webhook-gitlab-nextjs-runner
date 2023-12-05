// SPDX-FileCopyrightText: 2023 Isaak Tsalicoglou <isaak@waseigo.com>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var mu sync.Mutex
var stopSignal chan struct{}
var wg sync.WaitGroup
var prevCommitHash string

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

func getGitRepoPath() (string, error) {
	gitRepoPath := os.Getenv("GIT_REPO_PATH")
	return expandTilde(gitRepoPath)
}

func authenticate(w http.ResponseWriter, r *http.Request) error {
	apiKeyHeader := r.Header.Get("X-Gitlab-Token")
	fmt.Println("🤝 Received secret token:", apiKeyHeader)

	apiKey := os.Getenv("WEBHOOK_SECRET_TOKEN")
	if apiKeyHeader != apiKey {
		http.Error(w, "Invalid secret token!", http.StatusForbidden)
		return fmt.Errorf("🚫 Invalid secret token!")
	}
	return nil
}

func gitPull(gitRepoPath string) error {
	cmdGitPull := exec.Command("git", "pull")
	cmdGitPull.Dir = gitRepoPath
	stdout, err := cmdGitPull.StdoutPipe()
	if err != nil {
		return fmt.Errorf("💥 Error creating StdoutPipe for 'git pull': %v", err)
	}

	cmdGitPull.Stderr = os.Stderr
	err = cmdGitPull.Start()
	if err != nil {
		return fmt.Errorf("💥 Error starting 'git pull': %v", err)
	}

	outputBytes, err := ioutil.ReadAll(stdout)
	if err != nil {
		return fmt.Errorf("💥 Error reading 'git pull' output: %v", err)
	}

	err = cmdGitPull.Wait()
	if err != nil {
		return fmt.Errorf("💥 Error waiting for 'git pull': %v", err)
	}

	output := string(outputBytes)
	fmt.Println("Git pull output:", string(outputBytes))
	if strings.Contains(output, "Already up to date.") {
		fmt.Println("🤷 The pepository is already up to date. No further actions needed.")
		return nil
	}

	return nil
}

func npmStart(gitRepoPath string) error {
	cmd := exec.Command("npm", "start")
	cmd.Dir = gitRepoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Use a goroutine to wait for the command to complete
	wg.Add(1)
	go func() {
		defer wg.Done()

		err := cmd.Run()
		if err != nil {
			fmt.Println("💥 Error running 'npm start':", err)
		}
	}()

	return nil
}

func determineBuildRequired(gitRepoPath string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = gitRepoPath
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("💥 Error getting commit hash: %v", err)
	}

	commitHash := strings.TrimSpace(string(output))

	// Initialize prevCommitHash if it's the first time
	if prevCommitHash == "" {
		prevCommitHash = commitHash
	}

	if commitHash != prevCommitHash {
		prevCommitHash = commitHash
		return true, nil
	}

	return false, nil
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	// Read the request body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println("💥 Error reading request body:", err)
		http.Error(w, "💥 Error reading request body", http.StatusInternalServerError)
		return
	}

	err = authenticate(w, r)
	if err != nil {
		return
	}

	fmt.Fprintln(w, "Webhook request received") // Respond immediately with "OK"

	fmt.Println("\nWebhook payload:", string(body))

	go func() {
		fmt.Println("⚠️ 'git push' detected. Performing 'git pull' to see if an update is required.")

		err := updatePipeline()
		if err != nil {
			fmt.Println("💥 Error during update pipeline:", err)
			return
		}

		fmt.Println("Update process initiated!")
	}()
}

func checkGitChanges(gitRepoPath string) (bool, error) {
	cmd := exec.Command("git", "pull")
	cmd.Dir = gitRepoPath
	err := cmd.Run()
	if err != nil {
		return false, fmt.Errorf("💥 Error checking Git changes: %v", err)
	}

	return true, nil
}

func killNpmStartIfRunning() error {

	if stopSignal != nil {
		close(stopSignal)

		go func() {
			wg.Wait() // Wait for the npm start goroutine to complete
			fmt.Println("🪦 'npm start' was already running. Stopping it.")
		}()
	}

	// Kill npm start if the port is occupied
	port := os.Getenv("NEXTJS_PORT")
	if port == "" {
		port = "3000" // Default port if not provided in environment variables
	}

	killProcessOnPort(port)

	return nil
}

func getProcessIDOnPort(port string) (int, error) {

	// Extract PID of the running NextJS app from the output of netstat
	cmd := exec.Command("sh", "-c", fmt.Sprintf("netstat -tlpn | grep ':%s' | sed -E 's/^.* ([^\\/]+)\\/.*/\\1/'", port))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("💥 Error creating StdoutPipe: %v\n", err)
		return 0, err
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("💥 Error starting command: %v\n", err)
		return 0, err
	}

	output, err := ioutil.ReadAll(stdout)
	if err != nil {
		fmt.Printf("💥 Error reading output: %v\n", err)
		return 0, err
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf("💥 Error waiting for command: %v\n", err)
		return 0, err
	}

	if strings.TrimSpace(string(output)) == "" {
		fmt.Printf("🤷 No process found on port %s\n", port)
		return 0, nil
	}

	var pid int
	pid, err = strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		fmt.Printf("💥 Error parsing PID: %v\n", err)
		return 0, err
	}

	fmt.Println("ℹ️ Found PID:", pid)
	return pid, nil
}

func killProcessOnPort(port string) error {
	pid, err := getProcessIDOnPort(port)
	if err != nil {
		return err
	}

	if pid != 0 {
		fmt.Printf("🪦 Killing process with PID %d on port %s\n", pid, port)
		return exec.Command("kill", fmt.Sprintf("%d", pid)).Run()
	}

	fmt.Printf("🤷 No process found on port %s\n", port)
	return nil
}

func updatePipeline() error {
	mu.Lock()
	defer mu.Unlock()

	gitRepoPath, err := getGitRepoPath()
	if err != nil {
		return fmt.Errorf("💥 Error getting Git repository path: %v", err)
	}

	err = killNpmStartIfRunning()
	if err != nil {
		return fmt.Errorf("💥 Error killing already-running 'npm start': %v", err)
	}

	// Git pull check
	fmt.Println("📡 Performing 'git pull'…")

	err = gitPull(gitRepoPath)
	if err != nil {
		return fmt.Errorf("💥 Error during 'git pull': %v", err)
	}
	fmt.Println("Git pull completed")

	buildRequired, err := determineBuildRequired(gitRepoPath)
	if err != nil {
		return fmt.Errorf("💥 Error determining if 'npm build' is required: %v", err)
	}

	if buildRequired {
		// Run npm install and npm build
		fmt.Println("🛠️ Running 'npm install'…")
		cmdNpmInstall := exec.Command("npm", "install")
		cmdNpmInstall.Dir = gitRepoPath
		cmdNpmInstall.Stdout = os.Stdout
		cmdNpmInstall.Stderr = os.Stderr

		err := cmdNpmInstall.Run()
		if err != nil {
			return fmt.Errorf("💥 Error running npm install: %v", err)
		}

		fmt.Println("🏗️ Running 'npm build'…")
		cmdNpmBuild := exec.Command("npm", "build")
		cmdNpmBuild.Dir = gitRepoPath
		cmdNpmBuild.Stdout = os.Stdout
		cmdNpmBuild.Stderr = os.Stderr

		err = cmdNpmBuild.Run()
		if err != nil {
			return fmt.Errorf("Error running npm build: %v", err)
		}
		fmt.Println("🥳 Update completed (with 'npm build')!")
	} else {
		fmt.Println("😴 No changes in the Git repository since the last 'npm build'. Skipping update.")
		return nil
	}

	stopSignal = make(chan struct{})
	err = npmStart(gitRepoPath)
	if err != nil {
		return fmt.Errorf("💥 Error in 'npm start': %v", err)
	}

	fmt.Println("ℹ️ Update completed and 'npm start' issued.")
	return nil
}

func main() {
	fmt.Printf("💪 Initial setup…")
	err := updatePipeline()
	if err != nil {
		fmt.Println("💥 Error during initial setup:", err)
		return
	}

	port := os.Getenv("WEBHOOK_PORT")
	if port == "" {
		port = "8000"
	}

	fmt.Println("ℹ️ Starting the webhook server on port " + port)

	http.HandleFunc("/webhook", webhookHandler)

	err = http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("💥 Error starting the webhook server:", err)
	}
}
