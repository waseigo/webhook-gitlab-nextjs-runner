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
