#!/bin/sh

set -e

if [ ! -f "build/env.sh" ]; then
    echo "$0 must be run from the root of the repository."
    exit 2
fi

# Create fake Go workspace if it doesn't exist yet.
workspace="$PWD/build/_workspace"
root="$PWD"
truedir="$workspace/src/github.com/truechain"
if [ ! -L "$truedir/truechain-engineering-code" ]; then
    mkdir -p "$truedir"
    cd "$truedir"
    ln -s ../../../../../. truechain-engineering-code
    cd "$root"
fi

# Set up the environment to use the workspace.
GOPATH="$workspace"
export GOPATH

# Run the command inside the workspace.
cd "$truedir/truechain-engineering-code"
PWD="$truedir/truechain-engineering-code"

# Launch the arguments with the configured environment.
exec "$@"
