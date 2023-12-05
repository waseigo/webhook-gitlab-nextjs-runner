#!/bin/sh

# Webhooks of your gitlab repo are here https://gitlab.com/username/myproject/-/hooks
export WEBHOOK_SECRET_TOKEN="insert your the secret token of your repo's webhook for push events here"

# Default for the webhook is assumed to be 8000
export WEBHOOK_PORT="8000"

# Location of your git-cloned repo, e.g. "~/git/myproject"
export GIT_REPO_PATH="~/path/to/your/git/clone/of/the/nextjs/app"

# Default for NextJS is 3000
export NEXTJS_PORT="3000"

./webhook
