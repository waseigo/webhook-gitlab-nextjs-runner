# webhook-gitlab-nextjs-runner

I'm using this program to automatically rebuild and restart a NextJS app whenever a commit is pushed to the app's git repo on Gitlab.

## Build

`make build`

## Setup

First, on the machine where your NextJS app will run, do a `git clone` of your repo.

Edit `run_webhook.sh` and provide the following environment variables:
* `GIT_REPO_PATH`: the on-disk location of your cloned repo,
* `WEBHOOK_SECRET_TOKEN`: the secret token you generated on Gitlab for authorizing webhook requests,
* `WEBHOOK_PORT`: the port on which the webhook handler will run (default: 8000), and 
* `NEXTJS_PORT`: the port which NextJS will use (default: 3000).

Copy `run_webhook.sh` and the `webhook` binary somewhere on the same machine where the NextJS app will run.

Execute `./webhook`, perhaps in tmux or GNU Screen.

## Usage

Upon launch, the program performs a `git pull` and, if needed, will install dependencies, will build, and will finally start the NextJS app in a coroutine.

If your app has already been running, it keeps running, except if the `git pull` indicates that there are updates.

Each webhook request triggers a `git pull`. Upon that: if there are changes, the program terminates any running process occupying the specified port (default: 3000) or any already-running `npm start` command and does the following:
1. `npm install`
2. `npm run build`, and
3. `npm start` in a goroutine.

## Example output

```
2023-12-06T00:21:20+02:00       💪 Performing initial setup…
2023-12-06T00:21:20+02:00       🤷 No process found on port 3000
2023-12-06T00:21:20+02:00       📡 Performing 'git pull'…
2023-12-06T00:21:21+02:00       🤷 The repository is already up to date. No further actions needed.
2023-12-06T00:21:21+02:00       ✅ Git pull completed
2023-12-06T00:21:21+02:00       💪 Rebuilding is required!
2023-12-06T00:21:21+02:00       🛠️  Running 'npm install'…
2023-12-06T00:21:23+02:00       ✅ 'npm install' completed
2023-12-06T00:21:23+02:00       🏗️  Running 'npm run build'…
2023-12-06T00:21:52+02:00       ✅ 'npm run build' completed
2023-12-06T00:21:52+02:00       🥳 Update completed and 'npm start' issued.
2023-12-06T00:21:52+02:00       📟 Starting the webhook server on port 8000
2023-12-06T00:24:04+02:00       🤝 Received a valid secret token from Gitlab
2023-12-06T00:24:04+02:00       ⚠️ 'git push' detected. Performing 'git pull' to see if an update is required.
2023-12-06T00:24:04+02:00       📡 Performing 'git pull'…
2023-12-06T00:24:05+02:00       ✅ Git pull completed
2023-12-06T00:24:05+02:00       💪 Rebuilding is required!
2023-12-06T00:24:05+02:00       🛠️  Running 'npm install'…
2023-12-06T00:24:08+02:00       ✅ 'npm install' completed
2023-12-06T00:24:08+02:00       🏗️  Running 'npm run build'…
2023-12-06T00:24:42+02:00       ✅ 'npm run build' completed
2023-12-06T00:24:42+02:00       🥳 Update completed and 'npm start' issued.
```