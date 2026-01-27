#!/bin/bash
export PATH=$PATH:/usr/local/go/bin:/home/vamsi/go/bin
cd ~/projects/agenttask-operator
make manifests generate
