#!/usr/bin/env bash

# Get the commit message file path
COMMIT_MSG_FILE=$1

# Read the commit message
COMMIT_MSG=$(cat "$COMMIT_MSG_FILE")

# Bypass validation for merge/revert/amend/chore(deps) commits
if [[ "$COMMIT_MSG" =~ ^Merge.* || "$COMMIT_MSG" =~ ^Revert.* || "$COMMIT_MSG" =~ ^amend.* ]]; then
    exit 0
fi

# Regular expression for Conventional Commits
# type(scope): message OR type: message
CONVENTIONAL_COMMIT_REGEX="^(feat|fix|refactor|docs|test|chore|build|ci)(\([a-z0-9_-]+\))?!?: .+$"

if [[ ! "$COMMIT_MSG" =~ $CONVENTIONAL_COMMIT_REGEX ]]; then
    echo "====================================================================="
    echo "❌ ERROR: Invalid commit message format."
    echo "---------------------------------------------------------------------"
    echo "Your commit message:"
    echo "  \"$COMMIT_MSG\""
    echo "---------------------------------------------------------------------"
    echo "Commit message must follow Conventional Commits specification:"
    echo "  <type>(<scope>): <subject>  or  <type>: <subject>"
    echo ""
    echo "Allowed types: feat, fix, refactor, docs, test, chore, build, ci"
    echo "Example: feat(bootstrap): add load config function"
    echo "====================================================================="
    exit 1
fi

exit 0
